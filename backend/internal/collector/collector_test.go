package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/model"
)

type testStore struct {
	mu       sync.Mutex
	sources  []model.Source
	articles map[string][]model.Article
}

func (s *testStore) ListSources() []model.Source {
	return append([]model.Source(nil), s.sources...)
}

func (s *testStore) UpsertArticles(sourceID, _ string, articles []model.Article) (model.Source, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.articles[sourceID] = append([]model.Article(nil), articles...)
	for _, source := range s.sources {
		if source.ID == sourceID {
			return source, len(articles), nil
		}
	}
	return model.Source{}, 0, errors.New("source not found")
}

type testLoader struct {
	mu        sync.Mutex
	documents map[string]feed.Document
	errors    map[string]error
	loaded    []string
}

func (l *testLoader) Load(_ context.Context, rawURL string) (feed.Document, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loaded = append(l.loaded, rawURL)
	if err := l.errors[rawURL]; err != nil {
		return feed.Document{}, err
	}
	return l.documents[rawURL], nil
}

func TestRefreshAllUpdatesOnlyEnabledSources(t *testing.T) {
	repository := &testStore{
		sources: []model.Source{
			{ID: "enabled", Name: "Enabled Feed", URL: "https://example.com/enabled.xml", Enabled: true},
			{ID: "failed", Name: "Failed Feed", URL: "https://example.com/failed.xml", Enabled: true},
			{ID: "disabled", Name: "Disabled Feed", URL: "https://example.com/disabled.xml", Enabled: false},
		},
		articles: make(map[string][]model.Article),
	}
	loader := &testLoader{
		documents: map[string]feed.Document{
			"https://example.com/enabled.xml": {
				Format:  "rss",
				Entries: []feed.Entry{{ID: "feed-article", Title: "New article", Link: "https://example.com/article", Summary: "Summary"}},
			},
		},
		errors: map[string]error{"https://example.com/failed.xml": errors.New("unavailable")},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	result := New(repository, loader, 2, logger).RefreshAll(context.Background())

	if result.Sources != 2 || result.Refreshed != 1 || result.Added != 1 || result.Failed != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(loader.loaded) != 2 {
		t.Fatalf("unexpected loaded sources: %#v", loader.loaded)
	}
	if articles := repository.articles["enabled"]; len(articles) != 1 || articles[0].ID != "enabled-article" {
		t.Fatalf("unexpected articles: %#v", articles)
	}
	if _, loaded := repository.articles["disabled"]; loaded {
		t.Fatal("disabled source was refreshed")
	}
}

func TestRunRefreshesImmediatelyAtIntervalAndStops(t *testing.T) {
	repository := &testStore{
		sources:  []model.Source{{ID: "source", Name: "Source", URL: "https://example.com/feed.xml", Enabled: true}},
		articles: make(map[string][]model.Article),
	}
	loaded := make(chan struct{}, 2)
	loader := loaderFunc(func(context.Context, string) (feed.Document, error) {
		loaded <- struct{}{}
		return feed.Document{Format: "rss"}, nil
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		New(repository, loader, 1, logger).Run(ctx, 5*time.Millisecond)
		close(done)
	}()

	for attempt := 1; attempt <= 2; attempt++ {
		select {
		case <-loaded:
		case <-time.After(time.Second):
			t.Fatalf("refresh %d did not run", attempt)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
}

type loaderFunc func(context.Context, string) (feed.Document, error)

func (f loaderFunc) Load(ctx context.Context, rawURL string) (feed.Document, error) {
	return f(ctx, rawURL)
}
