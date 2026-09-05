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

	if result.Sources != 2 || result.Refreshed != 1 || result.Added != 1 || result.Failed != 1 || result.InitialRefreshed != 1 || result.InitialFailed != 1 || result.Retried != 1 || result.Recovered != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %#v, want one failure", result.Failures)
	}
	failure := result.Failures[0]
	if failure.SourceID != "failed" || failure.Name != "Failed Feed" || failure.URL != "https://example.com/failed.xml" || failure.Stage != "fetch" || failure.Reason != "unavailable" {
		t.Fatalf("failure = %#v", failure)
	}
	if len(loader.loaded) != 3 {
		t.Fatalf("unexpected loaded sources: %#v", loader.loaded)
	}
	if articles := repository.articles["enabled"]; len(articles) != 1 || articles[0].ID != "enabled-article" {
		t.Fatalf("unexpected articles: %#v", articles)
	}
	if _, loaded := repository.articles["disabled"]; loaded {
		t.Fatal("disabled source was refreshed")
	}
}

func TestRefreshAllRecoversFailedSourcesSequentially(t *testing.T) {
	repository := testRepository("first", "second", "third")
	loader := newScriptedLoader(func(_ context.Context, _ string, attempt int) (feed.Document, error) {
		if attempt == 1 {
			return feed.Document{}, errors.New("temporary failure")
		}
		return feed.Document{Format: "atom"}, nil
	})
	collector := New(repository, loader, 3, discardLogger())
	collector.retryInterval = 20 * time.Millisecond
	collector.retryTimeout = 100 * time.Millisecond
	collector.retryPhaseTimeout = time.Second

	result := collector.RefreshAll(context.Background())

	if result.Sources != 3 || result.InitialFailed != 3 || result.Retried != 3 || result.Recovered != 3 || result.Refreshed != 3 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	starts, maxActive := loader.retryDetails()
	if maxActive != 1 {
		t.Fatalf("maximum concurrent retries = %d, want 1", maxActive)
	}
	if len(starts) != 3 {
		t.Fatalf("retry starts = %v, want 3 entries", starts)
	}
	for index := 1; index < len(starts); index++ {
		if interval := starts[index].Sub(starts[index-1]); interval < 15*time.Millisecond {
			t.Fatalf("retry start interval = %v, want at least 15ms", interval)
		}
	}
}

func TestRefreshAllUsesLongerTimeoutForRetry(t *testing.T) {
	repository := testRepository("source")
	var retryDeadline time.Duration
	loader := newScriptedLoader(func(ctx context.Context, _ string, attempt int) (feed.Document, error) {
		if attempt == 1 {
			return feed.Document{}, errors.New("temporary failure")
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			return feed.Document{}, errors.New("retry context has no deadline")
		}
		retryDeadline = time.Until(deadline)
		return feed.Document{Format: "rss"}, nil
	})
	collector := New(repository, loader, 1, discardLogger())
	collector.initialTimeout = 10 * time.Millisecond
	collector.retryTimeout = 80 * time.Millisecond
	collector.retryInterval = 0

	result := collector.RefreshAll(context.Background())

	if result.Recovered != 1 || result.Failed != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if retryDeadline < 50*time.Millisecond || retryDeadline > 80*time.Millisecond {
		t.Fatalf("retry deadline = %v, want approximately 80ms", retryDeadline)
	}
}

func TestRefreshAllStopsStartingRetriesAfterPhaseTimeout(t *testing.T) {
	repository := testRepository("first", "second")
	loader := newScriptedLoader(func(ctx context.Context, _ string, attempt int) (feed.Document, error) {
		if attempt == 1 {
			return feed.Document{}, errors.New("temporary failure")
		}
		<-ctx.Done()
		return feed.Document{}, ctx.Err()
	})
	collector := New(repository, loader, 2, discardLogger())
	collector.retryTimeout = 30 * time.Millisecond
	collector.retryInterval = 0
	collector.retryPhaseTimeout = 10 * time.Millisecond

	result := collector.RefreshAll(context.Background())

	if result.Retried != 1 || result.Failed != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if attempts := loader.attemptCount("second"); attempts != 1 {
		t.Fatalf("second source attempts = %d, want only the initial attempt", attempts)
	}
	foundLimit := false
	for _, failure := range result.Failures {
		if failure.SourceID == "second" && failure.Reason == "retry phase time limit reached" {
			foundLimit = true
		}
	}
	if !foundLimit {
		t.Fatalf("time-limit failure not found: %+v", result.Failures)
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

type scriptedLoader struct {
	mu             sync.Mutex
	attempts       map[string]int
	retryStarts    []time.Time
	activeRetries  int
	maxActiveRetry int
	load           func(context.Context, string, int) (feed.Document, error)
}

func newScriptedLoader(load func(context.Context, string, int) (feed.Document, error)) *scriptedLoader {
	return &scriptedLoader{attempts: make(map[string]int), load: load}
}

func (l *scriptedLoader) Load(ctx context.Context, rawURL string) (feed.Document, error) {
	l.mu.Lock()
	l.attempts[rawURL]++
	attempt := l.attempts[rawURL]
	if attempt > 1 {
		l.retryStarts = append(l.retryStarts, time.Now())
		l.activeRetries++
		if l.activeRetries > l.maxActiveRetry {
			l.maxActiveRetry = l.activeRetries
		}
	}
	l.mu.Unlock()

	document, err := l.load(ctx, rawURL, attempt)

	if attempt > 1 {
		l.mu.Lock()
		l.activeRetries--
		l.mu.Unlock()
	}
	return document, err
}

func (l *scriptedLoader) retryDetails() ([]time.Time, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]time.Time(nil), l.retryStarts...), l.maxActiveRetry
}

func (l *scriptedLoader) attemptCount(sourceID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts["https://"+sourceID+".example/feed.xml"]
}

func testRepository(sourceIDs ...string) *testStore {
	sources := make([]model.Source, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		sources = append(sources, model.Source{ID: id, Name: id, URL: "https://" + id + ".example/feed.xml", Enabled: true})
	}
	return &testStore{sources: sources, articles: make(map[string][]model.Article)}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
