package collector

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/model"
)

const DefaultConcurrency = 8

type Store interface {
	ListSources() []model.Source
	UpsertArticles(string, string, []model.Article) (model.Source, int, error)
}

type Result struct {
	Sources   int       `json:"sources"`
	Refreshed int       `json:"refreshed"`
	Added     int       `json:"added"`
	Failed    int       `json:"failed"`
	Failures  []Failure `json:"failures"`
}

type Failure struct {
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Stage    string `json:"stage"`
	Reason   string `json:"reason"`
}

type Collector struct {
	store       Store
	loader      feed.Loader
	concurrency int
	logger      *slog.Logger
}

func New(repository Store, loader feed.Loader, concurrency int, logger *slog.Logger) *Collector {
	if concurrency < 1 {
		concurrency = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{store: repository, loader: loader, concurrency: concurrency, logger: logger}
}

func (c *Collector) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		c.logger.Error("feed collection is disabled because interval is not positive", "interval", interval)
		return
	}
	c.logger.Info("periodic feed collection started", "interval", interval, "concurrency", c.concurrency)
	for {
		startedAt := time.Now()
		result := c.RefreshAll(ctx)
		if ctx.Err() != nil {
			return
		}
		c.logger.Info("periodic feed collection completed",
			"sources", result.Sources,
			"refreshed", result.Refreshed,
			"added_articles", result.Added,
			"failed", result.Failed,
			"duration", time.Since(startedAt),
		)

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (c *Collector) RefreshAll(ctx context.Context) Result {
	sources := c.store.ListSources()
	enabled := make([]model.Source, 0, len(sources))
	for _, source := range sources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}
	result := Result{Sources: len(enabled), Failures: make([]Failure, 0)}
	if len(enabled) == 0 {
		return result
	}

	type outcome struct {
		source model.Source
		added  int
		stage  string
		err    error
	}
	jobs := make(chan model.Source, len(enabled))
	outcomes := make(chan outcome, len(enabled))
	for _, source := range enabled {
		jobs <- source
	}
	close(jobs)

	workerCount := min(c.concurrency, len(enabled))
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for source := range jobs {
				if err := ctx.Err(); err != nil {
					outcomes <- outcome{source: source, stage: "fetch", err: err}
					continue
				}
				document, err := c.loader.Load(ctx, source.URL)
				if err != nil {
					outcomes <- outcome{source: source, stage: "fetch", err: err}
					continue
				}
				_, added, err := c.store.UpsertArticles(source.ID, document.Format, feed.ArticlesFromDocument(source, document))
				outcomes <- outcome{source: source, added: added, stage: "save", err: err}
			}
		}()
	}
	workers.Wait()
	close(outcomes)

	for outcome := range outcomes {
		if outcome.err != nil {
			result.Failed++
			result.Failures = append(result.Failures, Failure{
				SourceID: outcome.source.ID,
				Name:     outcome.source.Name,
				URL:      outcome.source.URL,
				Stage:    outcome.stage,
				Reason:   outcome.err.Error(),
			})
			if ctx.Err() == nil {
				c.logger.Warn("periodic feed refresh failed", "source_id", outcome.source.ID, "source", outcome.source.Name, "error", outcome.err)
			}
			continue
		}
		result.Refreshed++
		result.Added += outcome.added
	}
	sort.Slice(result.Failures, func(i, j int) bool {
		return result.Failures[i].Name < result.Failures[j].Name
	})
	return result
}
