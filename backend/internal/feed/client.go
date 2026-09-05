package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxBytes       = 5 << 20
	DefaultRequestTimeout = 12 * time.Second
	maxRedirects          = 5
)

var (
	ErrUnsafeURL = errors.New("feed URL points to a blocked network address")
	ErrTooLarge  = errors.New("feed exceeds the maximum response size")
)

type Loader interface {
	Load(context.Context, string) (Document, error)
}

type Client struct {
	httpClient *http.Client
	maxBytes   int64
}

func NewClient() *Client {
	client := &Client{maxBytes: DefaultMaxBytes}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = client.dialContext
	client.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if len(previous) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return validateURL(request.Context(), request.URL)
		},
	}
	return client
}

func (c *Client) Load(ctx context.Context, rawURL string) (Document, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultRequestTimeout)
		defer cancel()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Document{}, fmt.Errorf("parse feed URL: %w", err)
	}
	if err := validateURL(ctx, parsed); err != nil {
		return Document{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Document{}, err
	}
	request.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/xml, text/xml;q=0.9, */*;q=0.1")
	request.Header.Set("User-Agent", "fliqrss/0.1")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Document{}, fmt.Errorf("fetch feed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Document{}, fmt.Errorf("fetch feed: unexpected HTTP status %d", response.StatusCode)
	}
	if response.ContentLength > c.maxBytes {
		return Document{}, ErrTooLarge
	}
	limited := io.LimitReader(response.Body, c.maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Document{}, fmt.Errorf("read feed: %w", err)
	}
	if int64(len(data)) > c.maxBytes {
		return Document{}, ErrTooLarge
	}
	document, err := Parse(data, response.Request.URL.String())
	if err != nil {
		return Document{}, fmt.Errorf("parse feed: %w", err)
	}
	return document, nil
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := resolvePublicAddresses(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var dialErrors []error
	for _, address := range addresses {
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, err)
	}
	return nil, errors.Join(dialErrors...)
}

func validateURL(ctx context.Context, target *url.URL) error {
	if target.Scheme != "http" && target.Scheme != "https" {
		return errors.New("feed URL must use HTTP or HTTPS")
	}
	if target.Hostname() == "" {
		return errors.New("feed URL must include a host")
	}
	if target.User != nil {
		return errors.New("feed URL must not include user information")
	}
	port := target.Port()
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return errors.New("feed URL contains an invalid port")
		}
	}
	_, err := resolvePublicAddresses(ctx, target.Hostname())
	return err
}

func resolvePublicAddresses(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if isBlockedAddress(address) {
			return nil, ErrUnsafeURL
		}
		return []netip.Addr{address.Unmap()}, nil
	}
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve feed host: %w", err)
	}
	public := make([]netip.Addr, 0, len(resolved))
	for _, address := range resolved {
		address = address.Unmap()
		if !isBlockedAddress(address) {
			public = append(public, address)
		}
	}
	if len(public) == 0 {
		return nil, ErrUnsafeURL
	}
	return public, nil
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isBlockedAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return true
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
