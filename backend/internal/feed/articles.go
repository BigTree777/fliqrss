package feed

import (
	"strings"
	"time"
	"unicode/utf8"

	"fliqrss/backend/internal/model"
)

func ArticlesFromDocument(source model.Source, document Document) []model.Article {
	articles := make([]model.Article, 0, len(document.Entries))
	for _, entry := range document.Entries {
		summary := entry.Summary
		if summary == "" {
			summary = entry.Content
		}
		body := []string{}
		if entry.Content != "" && entry.Content != summary {
			body = append(body, entry.Content)
		}
		publishedAt := entry.PublishedAt
		if publishedAt == "" {
			publishedAt = time.Now().UTC().Format(time.RFC3339)
		}
		articles = append(articles, model.Article{
			ID:             source.ID + "-" + strings.TrimPrefix(entry.ID, "feed-"),
			SourceID:       source.ID,
			Source:         source.Name,
			SourceInitials: sourceInitials(source.Name),
			PublishedAt:    publishedAt,
			ReadTime:       EstimateReadTime(summary, entry.Content),
			Title:          entry.Title,
			URL:            entry.Link,
			Summary:        summary,
			Body:           body,
			VisualLabel:    strings.ToUpper(document.Format) + " FEED",
			VisualTheme:    visualTheme(entry.ID),
		})
	}
	return articles
}

func sourceInitials(name string) string {
	words := strings.Fields(name)
	var initials []rune
	for _, word := range words {
		character, _ := utf8.DecodeRuneInString(word)
		if character != utf8.RuneError {
			initials = append(initials, character)
		}
		if len(initials) == 2 {
			break
		}
	}
	if len(initials) == 1 {
		runes := []rune(name)
		if len(runes) > 1 {
			initials = append(initials, runes[1])
		}
	}
	return strings.ToUpper(string(initials))
}

func visualTheme(identity string) string {
	themes := []string{"cobalt", "coral", "forest", "violet", "amber", "aqua"}
	value := 0
	for _, character := range identity {
		value = (value*31 + int(character)) % len(themes)
	}
	return themes[value]
}
