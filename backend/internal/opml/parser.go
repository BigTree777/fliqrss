package opml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const (
	DefaultMaxBytes    = 2 << 20
	DefaultMaxOutlines = 5000
	DefaultMaxDepth    = 12
	DefaultMaxFeeds    = 200
)

var (
	ErrInvalidDocument = errors.New("invalid OPML document")
	ErrLimitExceeded   = errors.New("OPML limit exceeded")
)

type Subscription struct {
	Title   string
	XMLURL  string
	HTMLURL string
	Type    string
	Tags    []string
}

type Limits struct {
	MaxOutlines int
	MaxDepth    int
	MaxFeeds    int
}

type outlineContext struct {
	tag string
}

func Parse(reader io.Reader) ([]Subscription, error) {
	return ParseWithLimits(reader, Limits{
		MaxOutlines: DefaultMaxOutlines,
		MaxDepth:    DefaultMaxDepth,
		MaxFeeds:    DefaultMaxFeeds,
	})
}

func ParseWithLimits(reader io.Reader, limits Limits) ([]Subscription, error) {
	decoder := xml.NewDecoder(reader)
	decoder.Strict = true
	rootSeen := false
	outlineCount := 0
	stack := make([]outlineContext, 0)
	subscriptions := make([]Subscription, 0)

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(element.Name.Local)
			if !rootSeen {
				rootSeen = true
				if name != "opml" {
					return nil, ErrInvalidDocument
				}
			}
			if name != "outline" {
				continue
			}

			outlineCount++
			if outlineCount > limits.MaxOutlines || len(stack)+1 > limits.MaxDepth {
				return nil, ErrLimitExceeded
			}
			title := firstAttribute(element.Attr, "title", "text")
			xmlURL := strings.TrimSpace(attribute(element.Attr, "xmlUrl"))
			htmlURL := strings.TrimSpace(attribute(element.Attr, "htmlUrl"))
			feedType := strings.TrimSpace(attribute(element.Attr, "type"))
			category := strings.TrimSpace(attribute(element.Attr, "category"))
			context := outlineContext{}
			if xmlURL == "" {
				context.tag = title
			} else {
				if len(subscriptions) >= limits.MaxFeeds {
					return nil, ErrLimitExceeded
				}
				subscriptions = append(subscriptions, Subscription{
					Title:   title,
					XMLURL:  xmlURL,
					HTMLURL: htmlURL,
					Type:    feedType,
					Tags:    mergeTags(ancestorTags(stack), decodeCategories(category)),
				})
			}
			stack = append(stack, context)
		case xml.EndElement:
			if strings.EqualFold(element.Name.Local, "outline") && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if !rootSeen {
		return nil, ErrInvalidDocument
	}
	return subscriptions, nil
}

func decodeCategories(category string) []string {
	if category == "" {
		return nil
	}
	result := make([]string, 0)
	for _, encoded := range strings.Split(category, ",") {
		decoded, err := url.PathUnescape(strings.TrimSpace(encoded))
		if err != nil {
			decoded = encoded
		}
		if decoded = strings.TrimSpace(decoded); decoded != "" {
			result = append(result, decoded)
		}
	}
	return result
}

func mergeTags(groups ...[]string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, name := range group {
			name = strings.TrimSpace(name)
			key := strings.ToLower(name)
			if name == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

func attribute(attributes []xml.Attr, name string) string {
	for _, candidate := range attributes {
		if strings.EqualFold(candidate.Name.Local, name) {
			return candidate.Value
		}
	}
	return ""
}

func firstAttribute(attributes []xml.Attr, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(attribute(attributes, name)); value != "" {
			return value
		}
	}
	return ""
}

func ancestorTags(stack []outlineContext) []string {
	result := make([]string, 0, len(stack))
	for _, context := range stack {
		result = append(result, context.tag)
	}
	return mergeTags(result)
}
