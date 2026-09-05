package feed

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidateURLRejectsPrivateAddresses(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/feed.xml",
		"http://10.0.0.1/feed.xml",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]/feed.xml",
	}
	for _, rawURL := range blocked {
		t.Run(rawURL, func(t *testing.T) {
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateURL(context.Background(), parsed); !errors.Is(err, ErrUnsafeURL) {
				t.Fatalf("validateURL() error = %v, want ErrUnsafeURL", err)
			}
		})
	}
}

func TestLoadRejectsLargeResponse(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://8.8.8.8/feed.xml", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		maxBytes: 8,
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("123456789")),
				Request:    request,
			}, nil
		})},
	}
	_, err = client.Load(context.Background(), request.URL.String())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Load() error = %v, want ErrTooLarge", err)
	}
}

func TestLoadParsesRSSResponse(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://8.8.8.8/feed.xml", nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `<rss version="2.0"><channel><title>Fetched feed</title><item><guid>1</guid><title>Fetched article</title></item></channel></rss>`
	client := &Client{
		maxBytes: DefaultMaxBytes,
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: int64(len(body)),
				Body:          io.NopCloser(strings.NewReader(body)),
				Request:       request,
			}, nil
		})},
	}
	document, err := client.Load(context.Background(), request.URL.String())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if document.Format != "rss" || len(document.Entries) != 1 || document.Entries[0].Title != "Fetched article" {
		t.Fatalf("document = %+v", document)
	}
}

func TestLoadKeepsCallerDeadline(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://8.8.8.8/feed.xml", nil)
	if err != nil {
		t.Fatal(err)
	}
	var remaining time.Duration
	client := &Client{
		maxBytes: DefaultMaxBytes,
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			deadline, ok := request.Context().Deadline()
			if !ok {
				return nil, errors.New("request context has no deadline")
			}
			remaining = time.Until(deadline)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`<rss version="2.0"><channel><title>Feed</title></channel></rss>`)),
				Request:    request,
			}, nil
		})},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := client.Load(ctx, request.URL.String()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if remaining < 70*time.Millisecond || remaining > 100*time.Millisecond {
		t.Fatalf("request deadline = %v, want approximately 100ms", remaining)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
