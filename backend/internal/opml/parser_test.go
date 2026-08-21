package opml

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseSubscriptionsAndFolderTags(t *testing.T) {
	document := `<?xml version="1.0"?>
<opml version="2.0"><body>
  <outline text="Technology">
    <outline title="Japanese">
      <outline type="rss" text="Example RSS" xmlUrl="https://example.test/rss.xml" htmlUrl="https://example.test/"/>
    </outline>
    <outline type="rss" title="Example Atom" xmlUrl="https://example.test/atom.xml"/>
  </outline>
</body></opml>`

	subscriptions, err := Parse(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 2 {
		t.Fatalf("subscription count = %d, want 2", len(subscriptions))
	}
	if got := subscriptions[0]; got.Title != "Example RSS" || got.XMLURL != "https://example.test/rss.xml" || len(got.Tags) != 2 || got.Tags[0] != "Technology" || got.Tags[1] != "Japanese" {
		t.Fatalf("first subscription = %+v", got)
	}
	if got := subscriptions[1]; len(got.Tags) != 1 || got.Tags[0] != "Technology" {
		t.Fatalf("second subscription = %+v", got)
	}
}

func TestMarshalRoundTripPreservesMultipleTags(t *testing.T) {
	original := []Subscription{{
		Title: "Example", XMLURL: "https://example.test/feed.xml", Type: "rss",
		Tags: []string{"Technology", "Japan, News", "開発/設計"},
	}}
	data, err := Marshal("Export", original, time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 || parsed[0].Title != original[0].Title || parsed[0].XMLURL != original[0].XMLURL || parsed[0].Type != "rss" {
		t.Fatalf("round trip subscription = %+v", parsed)
	}
	if len(parsed[0].Tags) != 3 || parsed[0].Tags[1] != "Japan, News" || parsed[0].Tags[2] != "開発/設計" {
		t.Fatalf("round trip tags = %v", parsed[0].Tags)
	}
}

func TestParseRejectsInvalidRoot(t *testing.T) {
	_, err := Parse(strings.NewReader(`<rss/>`))
	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf("Parse() error = %v, want ErrInvalidDocument", err)
	}
}

func TestParseEnforcesLimits(t *testing.T) {
	tests := []struct {
		name   string
		doc    string
		limits Limits
	}{
		{
			name:   "depth",
			doc:    `<opml><body><outline text="a"><outline text="b"><outline xmlUrl="https://example.test/feed"/></outline></outline></body></opml>`,
			limits: Limits{MaxOutlines: 10, MaxDepth: 2, MaxFeeds: 10},
		},
		{
			name:   "outlines",
			doc:    `<opml><body><outline/><outline/></body></opml>`,
			limits: Limits{MaxOutlines: 1, MaxDepth: 2, MaxFeeds: 10},
		},
		{
			name:   "feeds",
			doc:    `<opml><body><outline xmlUrl="https://example.test/1"/><outline xmlUrl="https://example.test/2"/></body></opml>`,
			limits: Limits{MaxOutlines: 10, MaxDepth: 2, MaxFeeds: 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseWithLimits(strings.NewReader(test.doc), test.limits)
			if !errors.Is(err, ErrLimitExceeded) {
				t.Fatalf("ParseWithLimits() error = %v, want ErrLimitExceeded", err)
			}
		})
	}
}
