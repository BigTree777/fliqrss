package opml

import (
	"encoding/xml"
	"net/url"
	"strings"
	"time"
)

type exportDocument struct {
	XMLName xml.Name   `xml:"opml"`
	Version string     `xml:"version,attr"`
	Head    exportHead `xml:"head"`
	Body    exportBody `xml:"body"`
}

type exportHead struct {
	Title       string `xml:"title"`
	DateCreated string `xml:"dateCreated"`
}

type exportBody struct {
	Outlines []exportOutline `xml:"outline"`
}

type exportOutline struct {
	Text     string `xml:"text,attr"`
	Title    string `xml:"title,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
	XMLURL   string `xml:"xmlUrl,attr"`
	HTMLURL  string `xml:"htmlUrl,attr,omitempty"`
	Category string `xml:"category,attr,omitempty"`
}

func Marshal(title string, subscriptions []Subscription, generatedAt time.Time) ([]byte, error) {
	if title = strings.TrimSpace(title); title == "" {
		title = "fliqrss subscriptions"
	}
	outlines := make([]exportOutline, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		outlines = append(outlines, exportOutline{
			Text:     subscription.Title,
			Title:    subscription.Title,
			Type:     subscription.Type,
			XMLURL:   subscription.XMLURL,
			HTMLURL:  subscription.HTMLURL,
			Category: encodeCategories(subscription.Tags),
		})
	}
	document := exportDocument{
		Version: "2.0",
		Head: exportHead{
			Title:       title,
			DateCreated: generatedAt.UTC().Format(time.RFC1123Z),
		},
		Body: exportBody{Outlines: outlines},
	}
	encoded, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), encoded...), nil
}

func encodeCategories(tags []string) string {
	encoded := make([]string, 0, len(tags))
	for _, tag := range mergeTags(tags) {
		encoded = append(encoded, url.PathEscape(tag))
	}
	return strings.Join(encoded, ",")
}
