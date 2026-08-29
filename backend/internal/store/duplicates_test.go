package store

import (
	"errors"
	"testing"

	"fliqrss/backend/internal/model"
)

func TestMemoryDetectsDuplicateArticlesAcrossSources(t *testing.T) {
	memory := NewMemory()
	firstSource, err := memory.CreateSource("First News", "https://first.example/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	secondSource, err := memory.CreateSource("Second News", "https://second.example/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}

	first := model.Article{
		ID:          "first-article",
		PublishedAt: "2026-08-29T08:00:00Z",
		Title:       "同じニュースを伝える十分に長いタイトル",
		URL:         "https://news.example/articles/42?utm_source=first",
	}
	second := model.Article{
		ID:          "second-article",
		PublishedAt: "2026-08-29T09:00:00Z",
		Title:       "別の見出しでもURLが同じ記事",
		URL:         "http://news.example/articles/42?utm_medium=rss",
	}
	if _, _, err := memory.UpsertArticles(firstSource.ID, "rss", []model.Article{first}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := memory.UpsertArticles(secondSource.ID, "rss", []model.Article{second}); err != nil {
		t.Fatal(err)
	}

	representative, err := memory.GetArticle(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if representative.DuplicateCount != 1 || len(representative.DuplicateSources) != 1 || representative.DuplicateSources[0] != secondSource.Name {
		t.Fatalf("representative duplicate metadata = %+v", representative)
	}
	duplicate, err := memory.GetArticle(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.DuplicateOfID != first.ID || duplicate.DuplicateReason != "url" {
		t.Fatalf("duplicate metadata = %+v", duplicate)
	}
	visible := memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != first.ID {
		t.Fatalf("visible articles = %+v, want only %s", visible, first.ID)
	}
	stats := memory.ArticleStats()
	if stats.Feed != 1 || stats.SourceFeedCounts[firstSource.ID] != 1 || stats.SourceFeedCounts[secondSource.ID] != 0 {
		t.Fatalf("duplicate-aware stats = %+v", stats)
	}
}

func TestMemoryUsesSourceOrderAndPreservesStateWhenRepresentativeChanges(t *testing.T) {
	memory := NewMemory()
	highPriority, _ := memory.CreateSource("High Priority", "https://high.example/feed.xml", "rss")
	lowPriority, _ := memory.CreateSource("Low Priority", "https://low.example/feed.xml", "rss")
	lowArticle := model.Article{ID: "low", Title: "Low", URL: "https://example.com/shared?utm_source=low"}
	highArticle := model.Article{ID: "high", Title: "High", URL: "http://example.com/shared"}

	if _, _, err := memory.UpsertArticles(lowPriority.ID, "rss", []model.Article{lowArticle}); err != nil {
		t.Fatal(err)
	}
	if _, err := memory.ApplyArticleAction(lowArticle.ID, model.ActionSave); err != nil {
		t.Fatal(err)
	}
	if _, _, err := memory.UpsertArticles(highPriority.ID, "rss", []model.Article{highArticle}); err != nil {
		t.Fatal(err)
	}

	visible := memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != highArticle.ID || !visible[0].State.Saved {
		t.Fatalf("priority representative = %+v", visible)
	}

	sources, err := memory.ReorderSources([]string{lowPriority.ID, highPriority.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sources[0].ID != lowPriority.ID {
		t.Fatalf("source order = %+v", sources)
	}
	visible = memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != highArticle.ID || !visible[0].State.Saved {
		t.Fatalf("reordering unexpectedly changed representative = %+v", visible)
	}
	if err := memory.ReconcileDuplicates(); err != nil {
		t.Fatal(err)
	}
	visible = memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != lowArticle.ID || !visible[0].State.Saved {
		t.Fatalf("reconciled representative = %+v", visible)
	}
	if _, err := memory.ReorderSources([]string{lowPriority.ID}); !errors.Is(err, ErrInvalidSourceOrder) {
		t.Fatalf("invalid source order error = %v", err)
	}
}

func TestMemoryPromotesDuplicateWhenRepresentativeSourceIsDeleted(t *testing.T) {
	memory := NewMemory()
	firstSource, _ := memory.CreateSource("First", "https://first.example/feed.xml", "rss")
	secondSource, _ := memory.CreateSource("Second", "https://second.example/feed.xml", "rss")
	first := model.Article{ID: "first", URL: "https://example.com/shared"}
	second := model.Article{ID: "second", URL: "https://example.com/shared?utm_source=second"}
	memory.UpsertArticles(firstSource.ID, "rss", []model.Article{first})
	memory.UpsertArticles(secondSource.ID, "rss", []model.Article{second})
	memory.ApplyArticleAction(first.ID, model.ActionFavorite)

	if err := memory.DeleteSource(firstSource.ID); err != nil {
		t.Fatal(err)
	}
	visible := memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != second.ID || !visible[0].State.Favorite {
		t.Fatalf("promoted representative = %+v", visible)
	}
}

func TestMemoryMergesExistingArticleStateWhenDuplicatesAreFirstDetected(t *testing.T) {
	memory := NewMemory()
	sources := []model.Source{{ID: "first-source", Name: "First"}, {ID: "second-source", Name: "Second"}}
	articles := []model.Article{
		{ID: "first", SourceID: sources[0].ID, URL: "https://example.com/shared", State: model.ArticleState{Saved: true}},
		{ID: "second", SourceID: sources[1].ID, URL: "https://example.com/shared?utm_source=second", State: model.ArticleState{Favorite: true, Deleted: true}},
	}
	memory.replace(nil, sources, articles)

	visible := memory.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != "first" || !visible[0].State.Saved || !visible[0].State.Favorite || visible[0].State.Deleted {
		t.Fatalf("merged representative = %+v", visible)
	}
}

func TestMemoryDoesNotDetectMatchingTitlesWithDifferentURLs(t *testing.T) {
	memory := NewMemory()
	firstSource, _ := memory.CreateSource("First", "https://first.example/feed.xml", "rss")
	secondSource, _ := memory.CreateSource("Second", "https://second.example/feed.xml", "rss")
	title := "製品発表についての同一タイトルです"

	articles := []struct {
		sourceID string
		article  model.Article
	}{
		{firstSource.ID, model.Article{ID: "first", PublishedAt: "2026-08-29T08:00:00Z", Title: title, URL: "https://first.example/1"}},
		{secondSource.ID, model.Article{ID: "near", PublishedAt: "2026-08-30T08:00:00Z", Title: " 製品発表についての同一タイトルです。 ", URL: "https://second.example/2"}},
	}
	for _, item := range articles {
		if _, _, err := memory.UpsertArticles(item.sourceID, "rss", []model.Article{item.article}); err != nil {
			t.Fatal(err)
		}
	}

	for _, article := range memory.ListArticles(model.ArticleFilter{State: "all"}) {
		if article.DuplicateOfID != "" || article.DuplicateCount != 0 {
			t.Fatalf("matching title was incorrectly detected as duplicate: %+v", article)
		}
	}
}

func TestMemoryDoesNotDetectDuplicateWithinSameSource(t *testing.T) {
	memory := NewMemory()
	source, _ := memory.CreateSource("Only Source", "https://example.com/feed.xml", "rss")
	articles := []model.Article{
		{ID: "first", PublishedAt: "2026-08-29T08:00:00Z", Title: "同一ソースの十分に長いタイトル", URL: "https://example.com/article"},
		{ID: "second", PublishedAt: "2026-08-29T09:00:00Z", Title: "同一ソースの十分に長いタイトル", URL: "https://example.com/article?utm_source=rss"},
	}
	if _, _, err := memory.UpsertArticles(source.ID, "rss", articles); err != nil {
		t.Fatal(err)
	}
	for _, article := range memory.ListArticles(model.ArticleFilter{State: "all"}) {
		if article.DuplicateOfID != "" || article.DuplicateCount != 0 {
			t.Fatalf("same-source article was incorrectly detected as duplicate: %+v", article)
		}
	}
}

func TestCanonicalArticleURL(t *testing.T) {
	first := canonicalArticleURL("https://Example.com:443/news/1/?b=2&utm_source=rss&a=1#section")
	second := canonicalArticleURL("http://example.com/news/1?a=1&b=2")
	if first != second {
		t.Fatalf("canonical URLs differ: %q != %q", first, second)
	}
}
