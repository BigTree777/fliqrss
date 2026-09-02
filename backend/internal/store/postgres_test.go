package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"fliqrss/backend/internal/model"
)

func TestPostgreSQLPersistsData(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := fmt.Sprintf("fliqrss_test_%d", time.Now().UnixNano())
	admin, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()

	isolatedURL, err := databaseURLWithSearchPath(databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := OpenPostgreSQL(ctx, isolatedURL)
	if err != nil {
		t.Fatal(err)
	}

	tag, err := repository.CreateTag("テクノロジー")
	if err != nil {
		t.Fatal(err)
	}
	source, err := repository.CreateSource("Example Feed", "https://example.com/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetSourceTags(source.ID, []string{tag.ID}); err != nil {
		t.Fatal(err)
	}
	article := model.Article{
		ID:             "persistent-article",
		SourceInitials: "EF",
		PublishedAt:    "2026-08-21T00:00:00Z",
		ReadTime:       3,
		Title:          "永続化を確認する記事",
		URL:            "https://example.com/articles/1",
		Summary:        "要約",
		Body:           []string{"本文1", "本文2"},
	}
	if _, added, err := repository.UpsertArticles(source.ID, "rss", []model.Article{article}); err != nil || added != 1 {
		t.Fatalf("upsert article: added=%d, err=%v", added, err)
	}
	for _, action := range []model.ArticleAction{model.ActionSkip, model.ActionSave, model.ActionFavorite} {
		if _, err := repository.ApplyArticleAction(article.ID, action); err != nil {
			t.Fatal(err)
		}
	}
	secondSource, err := repository.CreateSource("Higher Priority Feed", "https://higher.example/feed.xml", "rss")
	if err != nil {
		t.Fatal(err)
	}
	duplicateArticle := article
	duplicateArticle.ID = "higher-priority-article"
	duplicateArticle.URL = "http://example.com/articles/1?utm_source=higher"
	if _, added, err := repository.UpsertArticles(secondSource.ID, "rss", []model.Article{duplicateArticle}); err != nil || added != 1 {
		t.Fatalf("upsert duplicate article: added=%d, err=%v", added, err)
	}
	if _, err := repository.ReorderSources([]string{secondSource.ID, source.ID}); err != nil {
		t.Fatal(err)
	}
	visible := repository.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != article.ID {
		t.Fatalf("reorder changed representative before refresh: %+v", visible)
	}
	if err := repository.ReconcileDuplicates(); err != nil {
		t.Fatal(err)
	}
	visible = repository.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != duplicateArticle.ID || !visible[0].State.Saved {
		t.Fatalf("visible representative before reload: %+v", visible)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err = OpenPostgreSQL(ctx, isolatedURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	visible = repository.ListArticles(model.ArticleFilter{State: "all"})
	if len(visible) != 1 || visible[0].ID != duplicateArticle.ID {
		t.Fatalf("visible representative after reload: %+v", visible)
	}
	restored := visible[0]
	if !restored.State.Read || !restored.State.Skipped || !restored.State.Saved || !restored.State.Favorite {
		t.Fatalf("article state was not persisted: %+v", restored.State)
	}
	if len(restored.Body) != 2 || restored.Body[1] != "本文2" {
		t.Fatalf("article body was not persisted: %#v", restored.Body)
	}
	restoredSources := repository.ListSources()
	if len(restoredSources) != 2 || restoredSources[0].ID != secondSource.ID || restoredSources[1].ID != source.ID {
		t.Fatalf("source order was not persisted: %+v", restoredSources)
	}
	restoredSource, err := repository.GetSource(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredSource.ArticleCount != 1 || len(restoredSource.TagIDs) != 1 || restoredSource.TagIDs[0] != tag.ID {
		t.Fatalf("source relations were not persisted: %+v", restoredSource)
	}
	if count, err := repository.ResetSkipped(secondSource.ID); err != nil || count != 1 {
		t.Fatalf("reset skipped: count=%d, err=%v", count, err)
	}
	restored, err = repository.GetArticle(duplicateArticle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State.Read || restored.State.Skipped || !restored.State.Saved || !restored.State.Favorite {
		t.Fatalf("reset changed unrelated article state: %+v", restored.State)
	}
	unread := model.Article{
		ID:    "unread-persistent-article",
		Title: "一括既読を確認する記事",
		URL:   "https://example.com/articles/unread",
	}
	if _, added, err := repository.UpsertArticles(source.ID, "rss", []model.Article{unread}); err != nil || added != 1 {
		t.Fatalf("upsert unread article: added=%d, err=%v", added, err)
	}
	if count, err := repository.MarkAllRead(source.ID); err != nil || count != 1 {
		t.Fatalf("mark all read: count=%d, err=%v", count, err)
	}
	unread, err = repository.GetArticle(unread.ID)
	if err != nil || !unread.State.Read {
		t.Fatalf("unread article was not marked read: %+v, err=%v", unread.State, err)
	}
}

func databaseURLWithSearchPath(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
