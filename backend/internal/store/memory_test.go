package store

import (
	"testing"

	"fliqrss/backend/internal/model"
)

func TestNewMemoryStartsEmpty(t *testing.T) {
	memory := NewMemory()
	if articles := memory.ListArticles(model.ArticleFilter{State: "all"}); len(articles) != 0 {
		t.Fatalf("initial articles = %d, want 0", len(articles))
	}
	if sources := memory.ListSources(); len(sources) != 0 {
		t.Fatalf("initial sources = %d, want 0", len(sources))
	}
	if tags := memory.ListTags(); len(tags) != 0 {
		t.Fatalf("initial tags = %d, want 0", len(tags))
	}
}

func TestMarkAllReadOnlyMarksVisibleFeedArticles(t *testing.T) {
	memory := NewMemory()
	source, err := memory.CreateSource("Example", "https://example.com/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	articles := []model.Article{
		{ID: "unread", URL: "https://example.com/unread"},
		{ID: "favorite", URL: "https://example.com/favorite"},
		{ID: "saved", URL: "https://example.com/saved"},
		{ID: "deleted", URL: "https://example.com/deleted"},
		{ID: "skipped", URL: "https://example.com/skipped"},
	}
	if _, _, err := memory.UpsertArticles(source.ID, "rss", articles); err != nil {
		t.Fatal(err)
	}
	for id, action := range map[string]model.ArticleAction{
		"favorite": model.ActionFavorite,
		"saved":    model.ActionSave,
		"deleted":  model.ActionDelete,
		"skipped":  model.ActionSkip,
	} {
		if _, err := memory.ApplyArticleAction(id, action); err != nil {
			t.Fatal(err)
		}
	}

	count, err := memory.MarkAllRead()
	if err != nil || count != 2 {
		t.Fatalf("mark all read: count=%d, err=%v", count, err)
	}
	for _, id := range []string{"unread", "favorite"} {
		article, err := memory.GetArticle(id)
		if err != nil || !article.State.Read {
			t.Fatalf("article %s was not marked read: %+v, err=%v", id, article.State, err)
		}
	}
	for _, id := range []string{"saved", "deleted"} {
		article, err := memory.GetArticle(id)
		if err != nil || article.State.Read {
			t.Fatalf("article %s unexpectedly changed: %+v, err=%v", id, article.State, err)
		}
	}
	skipped, err := memory.GetArticle("skipped")
	if err != nil || !skipped.State.Read || !skipped.State.Skipped {
		t.Fatalf("skipped article unexpectedly changed: %+v, err=%v", skipped.State, err)
	}
}
