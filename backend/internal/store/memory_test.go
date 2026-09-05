package store

import (
	"testing"
	"time"

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

func TestPublishedAfterFiltersFeedStatsAndMarkAllRead(t *testing.T) {
	memory := NewMemory()
	source, err := memory.CreateSource("Example", "https://example.com/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	articles := []model.Article{
		{ID: "recent", PublishedAt: cutoff.Add(time.Hour).Format(time.RFC3339), URL: "https://example.com/recent"},
		{ID: "old", PublishedAt: cutoff.Format(time.RFC3339), URL: "https://example.com/old"},
	}
	if _, _, err := memory.UpsertArticles(source.ID, "rss", articles); err != nil {
		t.Fatal(err)
	}

	filter := model.ArticleFilter{State: "feed", PublishedAfter: cutoff}
	page, err := memory.ListArticlePage(filter, "", 20)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "recent" {
		t.Fatalf("filtered page = %+v, err=%v", page, err)
	}
	stats := memory.ArticleStats(cutoff)
	if stats.Feed != 1 || stats.SourceFeedCounts[source.ID] != 1 {
		t.Fatalf("filtered stats = %+v", stats)
	}
	if count, err := memory.MarkAllRead(source.ID, cutoff); err != nil || count != 1 {
		t.Fatalf("mark all read: count=%d, err=%v", count, err)
	}
	old, err := memory.GetArticle("old")
	if err != nil || old.State.Read {
		t.Fatalf("old article was unexpectedly marked read: %+v, err=%v", old.State, err)
	}
}

func TestListTagsIncludesUsageAndLastUsedAt(t *testing.T) {
	memory := NewMemory()
	firstSource, err := memory.CreateSource("First", "https://example.com/first.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	secondSource, err := memory.CreateSource("Second", "https://example.com/second.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	frequentTag, err := memory.CreateTag("Frequent")
	if err != nil {
		t.Fatal(err)
	}
	recentTag, err := memory.CreateTag("Recent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memory.SetSourceTags(firstSource.ID, []string{frequentTag.ID, recentTag.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.SetSourceTags(secondSource.ID, []string{frequentTag.ID}); err != nil {
		t.Fatal(err)
	}

	tags := memory.ListTags()
	if len(tags) != 2 {
		t.Fatalf("tags = %d, want 2", len(tags))
	}
	if tags[0].UsageCount != 2 || tags[0].LastUsedAt == nil {
		t.Fatalf("frequent tag metadata = %+v", tags[0])
	}
	if tags[1].UsageCount != 1 || tags[1].LastUsedAt == nil {
		t.Fatalf("recent tag metadata = %+v", tags[1])
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

	count, err := memory.MarkAllRead(source.ID, time.Time{})
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
