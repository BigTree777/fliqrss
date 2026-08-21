package feed

import (
	"testing"

	"fliqrss/backend/internal/model"
)

func TestArticlesFromDocument(t *testing.T) {
	source := model.Source{ID: "source-1", Name: "Example Feed"}
	document := Document{
		Format: "atom",
		Entries: []Entry{{
			ID:          "feed-entry-1",
			Title:       "Article title",
			Link:        "https://example.com/articles/1",
			PublishedAt: "2026-08-21T00:00:00Z",
			Summary:     "Article summary",
			Content:     "Article content",
		}},
	}

	articles := ArticlesFromDocument(source, document)

	if len(articles) != 1 {
		t.Fatalf("article count = %d, want 1", len(articles))
	}
	article := articles[0]
	if article.ID != "source-1-entry-1" || article.SourceID != source.ID || article.SourceInitials != "EF" {
		t.Fatalf("article identity = %+v", article)
	}
	if article.Summary != "Article summary" || len(article.Body) != 1 || article.Body[0] != "Article content" {
		t.Fatalf("article text = %+v", article)
	}
}

func TestArticlesFromDocumentUsesContentAsSummary(t *testing.T) {
	articles := ArticlesFromDocument(model.Source{ID: "source", Name: "日本語"}, Document{
		Format:  "rss",
		Entries: []Entry{{ID: "entry", Content: "Full content"}},
	})

	if len(articles) != 1 || articles[0].Summary != "Full content" || len(articles[0].Body) != 0 {
		t.Fatalf("article = %+v", articles)
	}
}
