package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrUnsupportedFormat = errors.New("unsupported feed format")

type Document struct {
	Format  string
	Title   string
	Entries []Entry
}

type Entry struct {
	ID          string
	Title       string
	Link        string
	PublishedAt string
	Summary     string
	Content     string
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PublishedAt string `xml:"pubDate"`
	Description string `xml:"description"`
	Content     string `xml:"encoded"`
}

type atomDocument struct {
	Title   atomText   `xml:"title"`
	Entries []atomItem `xml:"entry"`
}

type atomItem struct {
	ID        string     `xml:"id"`
	Title     atomText   `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   atomText   `xml:"summary"`
	Content   atomText   `xml:"content"`
}

type atomText struct {
	Type  string `xml:"type,attr"`
	Inner string `xml:",innerxml"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

var tagPattern = regexp.MustCompile(`<[^>]+>`)
var spaceBeforePunctuation = regexp.MustCompile(`\s+([.,!?;:、。！？])`)

func Parse(data []byte, baseURL string) (Document, error) {
	root, err := rootElement(data)
	if err != nil {
		return Document{}, err
	}
	switch strings.ToLower(root.Local) {
	case "rss":
		return parseRSS(data, baseURL)
	case "feed":
		return parseAtom(data, baseURL)
	default:
		return Document{}, ErrUnsupportedFormat
	}
}

func rootElement(data []byte) (xml.Name, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.Name{}, err
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name, nil
		}
	}
}

func parseRSS(data []byte, baseURL string) (Document, error) {
	var source rssDocument
	if err := xml.Unmarshal(data, &source); err != nil {
		return Document{}, err
	}
	result := Document{Format: "rss", Title: cleanText(source.Channel.Title)}
	result.Entries = make([]Entry, 0, len(source.Channel.Items))
	for _, item := range source.Channel.Items {
		summary := cleanText(item.Description)
		content := cleanText(item.Content)
		if content == summary {
			content = ""
		}
		link := resolveLink(baseURL, strings.TrimSpace(item.Link))
		identity := firstNonEmpty(strings.TrimSpace(item.GUID), link, cleanText(item.Title)+"|"+item.PublishedAt)
		result.Entries = append(result.Entries, Entry{
			ID:          stableID(identity),
			Title:       cleanText(item.Title),
			Link:        link,
			PublishedAt: normalizeTime(item.PublishedAt),
			Summary:     summary,
			Content:     content,
		})
	}
	return result, nil
}

func parseAtom(data []byte, baseURL string) (Document, error) {
	var source atomDocument
	if err := xml.Unmarshal(data, &source); err != nil {
		return Document{}, err
	}
	result := Document{Format: "atom", Title: cleanText(source.Title.Inner)}
	result.Entries = make([]Entry, 0, len(source.Entries))
	for _, item := range source.Entries {
		published := firstNonEmpty(item.Published, item.Updated)
		link := atomEntryLink(baseURL, item.Links)
		summary := cleanText(item.Summary.Inner)
		content := cleanText(item.Content.Inner)
		if content == summary {
			content = ""
		}
		identity := firstNonEmpty(strings.TrimSpace(item.ID), link, cleanText(item.Title.Inner)+"|"+published)
		result.Entries = append(result.Entries, Entry{
			ID:          stableID(identity),
			Title:       cleanText(item.Title.Inner),
			Link:        link,
			PublishedAt: normalizeTime(published),
			Summary:     summary,
			Content:     content,
		})
	}
	return result, nil
}

func atomEntryLink(baseURL string, links []atomLink) string {
	for _, link := range links {
		if link.Rel == "alternate" || link.Rel == "" {
			return resolveLink(baseURL, link.Href)
		}
	}
	if len(links) > 0 {
		return resolveLink(baseURL, links[0].Href)
	}
	return ""
}

func resolveLink(baseURL, rawLink string) string {
	if rawLink == "" {
		return ""
	}
	link, err := url.Parse(rawLink)
	if err != nil {
		return rawLink
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return rawLink
	}
	return base.ResolveReference(link).String()
}

func cleanText(value string) string {
	return cleanTextDepth(value, 0)
}

func cleanTextDepth(value string, depth int) string {
	value = strings.TrimSpace(html.UnescapeString(value))
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "<") {
		return strings.Join(strings.Fields(value), " ")
	}
	decoder := xml.NewDecoder(strings.NewReader("<root>" + value + "</root>"))
	var builder strings.Builder
	validXML := true
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			validXML = false
			break
		}
		if text, ok := token.(xml.CharData); ok {
			if builder.Len() > 0 && !strings.HasSuffix(builder.String(), " ") {
				builder.WriteByte(' ')
			}
			builder.Write(bytesTrimSpace(text))
		}
	}
	if cleaned := strings.Join(strings.Fields(builder.String()), " "); validXML && cleaned != "" {
		if depth < 3 && cleaned != value && strings.Contains(cleaned, "<") {
			return cleanTextDepth(cleaned, depth+1)
		}
		return spaceBeforePunctuation.ReplaceAllString(cleaned, "$1")
	}
	cleaned := strings.Join(strings.Fields(html.UnescapeString(tagPattern.ReplaceAllString(value, " "))), " ")
	return spaceBeforePunctuation.ReplaceAllString(cleaned, "$1")
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

func normalizeTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
	}
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

func stableID(identity string) string {
	if identity == "" {
		identity = time.Now().UTC().Format(time.RFC3339Nano)
	}
	digest := sha256.Sum256([]byte(identity))
	return "feed-" + hex.EncodeToString(digest[:12])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func EstimateReadTime(values ...string) int {
	characters := 0
	for _, value := range values {
		characters += utf8.RuneCountInString(value)
	}
	minutes := (characters + 399) / 400
	if minutes < 1 {
		return 1
	}
	return minutes
}
