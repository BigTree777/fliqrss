package feed

import (
	"errors"
	"testing"
)

func TestParseRSS(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Example RSS</title>
    <item>
      <title>RSS article</title>
      <link>/articles/1</link>
      <guid>rss-1</guid>
      <pubDate>Thu, 21 Aug 2025 09:00:00 +0000</pubDate>
      <description><![CDATA[<p>Short summary.</p>]]></description>
      <content:encoded><![CDATA[<p>Full <strong>content</strong>.</p>]]></content:encoded>
    </item>
  </channel>
</rss>`)

	document, err := Parse(data, "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if document.Format != "rss" || document.Title != "Example RSS" || len(document.Entries) != 1 {
		t.Fatalf("document = %+v", document)
	}
	entry := document.Entries[0]
	if entry.Title != "RSS article" || entry.Link != "https://example.com/articles/1" {
		t.Fatalf("entry title/link = %+v", entry)
	}
	if entry.Summary != "Short summary." || entry.Content != "Full content." {
		t.Fatalf("entry text = %+v", entry)
	}
	if entry.PublishedAt != "2025-08-21T09:00:00Z" {
		t.Fatalf("publishedAt = %q", entry.PublishedAt)
	}
}

func TestParseAtom(t *testing.T) {
	data := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <entry>
    <title>Atom article</title>
    <id>urn:uuid:atom-1</id>
    <updated>2025-08-21T10:00:00+09:00</updated>
    <link rel="alternate" href="/posts/1" />
    <summary type="html">&lt;p&gt;Atom summary.&lt;/p&gt;</summary>
    <content type="html">&lt;p&gt;Atom full content.&lt;/p&gt;</content>
  </entry>
</feed>`)

	document, err := Parse(data, "https://example.com/atom.xml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if document.Format != "atom" || document.Title != "Example Atom" || len(document.Entries) != 1 {
		t.Fatalf("document = %+v", document)
	}
	entry := document.Entries[0]
	if entry.Link != "https://example.com/posts/1" || entry.Summary != "Atom summary." || entry.Content != "Atom full content." {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.PublishedAt != "2025-08-21T01:00:00Z" {
		t.Fatalf("publishedAt = %q", entry.PublishedAt)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	_, err := Parse([]byte(`<html><body>Not a feed</body></html>`), "https://example.com")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("Parse() error = %v, want ErrUnsupportedFormat", err)
	}
}

func TestCleanTextPreservesAmpersand(t *testing.T) {
	if got := cleanText("Research & Development"); got != "Research & Development" {
		t.Fatalf("cleanText() = %q", got)
	}
	if got := cleanText("<p>Research & Development</p>"); got != "Research & Development" {
		t.Fatalf("cleanText() malformed HTML fallback = %q", got)
	}
}
