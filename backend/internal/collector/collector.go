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

const (
	DefaultConcurrency    = 8
	initialAttemptTimeout = feed.DefaultRequestTimeout
	retryAttemptTimeout   = 60 * time.Second
	retryStartInterval    = 5 * time.Second
	retryPhaseTimeout     = 5 * time.Minute
)

type Store interface {
	ListSources() []model.Source
	UpsertArticles(string, string, []model.Article) (model.Source, int, error)
}

type Result struct {
	Sources          int       `json:"sources"`
	Refreshed        int       `json:"refreshed"`
	Added            int       `json:"added"`
	Failed           int       `json:"failed"`
	InitialRefreshed int       `json:"initialRefreshed"`
	InitialFailed    int       `json:"initialFailed"`
	Retried          int       `json:"retried"`
	Recovered        int       `json:"recovered"`
	Failures         []Failure `json:"failures"`
}

type Failure struct {
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Stage    string `json:"stage"`
	Reason   string `json:"reason"`
}

type Collector struct {
	store             Store
	loader            feed.Loader
	concurrency       int
	initialTimeout    time.Duration
	retryTimeout      time.Duration
	retryInterval     time.Duration
	retryPhaseTimeout time.Duration
	logger            *slog.Logger
}

func New(repository Store, loader feed.Loader, concurrency int, logger *slog.Logger) *Collector {
	if concurrency < 1 {
		concurrency = 1
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		store:             repository,
		loader:            loader,
		concurrency:       concurrency,
		initialTimeout:    initialAttemptTimeout,
		retryTimeout:      retryAttemptTimeout,
		retryInterval:     retryStartInterval,
		retryPhaseTimeout: retryPhaseTimeout,
		logger:            logger,
	}
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
			"retried", result.Retried,
			"recovered", result.Recovered,
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

	jobs := make(chan model.Source, len(enabled))
	outcomes := make(chan refreshOutcome, len(enabled))
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
					outcomes <- refreshOutcome{source: source, stage: "fetch", err: err}
					continue
				}
				outcomes <- c.refreshSource(ctx, source, c.initialTimeout)
			}
		}()
	}
	workers.Wait()
	close(outcomes)

	initialFailures := make(map[string]refreshOutcome)
	for outcome := range outcomes {
		if outcome.err != nil {
			result.InitialFailed++
			initialFailures[outcome.source.ID] = outcome
			continue
		}
		result.InitialRefreshed++
		result.Refreshed++
		result.Added += outcome.added
	}

	if ctx.Err() == nil && len(initialFailures) > 0 {
		c.retryFailures(ctx, enabled, initialFailures, &result)
	} else {
		for _, source := range enabled {
			if outcome, failed := initialFailures[source.ID]; failed {
				result.Failures = append(result.Failures, failureFromOutcome(outcome))
			}
		}
	}
	result.Failed = len(result.Failures)
	sort.Slice(result.Failures, func(i, j int) bool {
		return result.Failures[i].Name < result.Failures[j].Name
	})
	return result
}

type refreshOutcome struct {
	source model.Source
	added  int
	stage  string
	err    error
}

func (c *Collector) refreshSource(ctx context.Context, source model.Source, timeout time.Duration) refreshOutcome {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	document, err := c.loader.Load(attemptCtx, source.URL)
	if err != nil {
		return refreshOutcome{source: source, stage: "fetch", err: err}
	}
	_, added, err := c.store.UpsertArticles(source.ID, document.Format, feed.ArticlesFromDocument(source, document))
	return refreshOutcome{source: source, added: added, stage: "save", err: err}
}

func (c *Collector) retryFailures(ctx context.Context, sources []model.Source, failures map[string]refreshOutcome, result *Result) {
	phaseStartedAt := time.Now()
	var previousStartedAt time.Time
	for index, source := range sources {
		initialOutcome, failed := failures[source.ID]
		if !failed {
			continue
		}
		if c.retryPhaseExpired(phaseStartedAt) {
			c.appendRetryLimitFailures(sources[index:], failures, result)
			return
		}
		if !previousStartedAt.IsZero() {
			wait := c.retryInterval - time.Since(previousStartedAt)
			if wait > 0 {
				if c.retryPhaseTimeout > 0 && time.Since(phaseStartedAt)+wait >= c.retryPhaseTimeout {
					c.appendRetryLimitFailures(sources[index:], failures, result)
					return
				}
				if !waitForContext(ctx, wait) {
					c.appendRemainingFailures(sources[index:], failures, result)
					return
				}
			}
		}
		if err := ctx.Err(); err != nil {
			c.appendRemainingFailures(sources[index:], failures, result)
			return
		}

		previousStartedAt = time.Now()
		result.Retried++
		outcome := c.refreshSource(ctx, source, c.retryTimeout)
		if outcome.err == nil {
			result.Recovered++
			result.Refreshed++
			result.Added += outcome.added
			continue
		}
		result.Failures = append(result.Failures, failureFromOutcome(outcome))
		if ctx.Err() == nil {
			c.logger.Warn("feed refresh failed after retry", "source_id", source.ID, "source", source.Name, "initial_error", initialOutcome.err, "error", outcome.err)
		}
	}
}

func (c *Collector) retryPhaseExpired(startedAt time.Time) bool {
	return c.retryPhaseTimeout > 0 && time.Since(startedAt) >= c.retryPhaseTimeout
}

func (c *Collector) appendRetryLimitFailures(sources []model.Source, failures map[string]refreshOutcome, result *Result) {
	for _, source := range sources {
		outcome, failed := failures[source.ID]
		if !failed {
			continue
		}
		result.Failures = append(result.Failures, Failure{
			SourceID: source.ID,
			Name:     source.Name,
			URL:      source.URL,
			Stage:    outcome.stage,
			Reason:   "retry phase time limit reached",
		})
		c.logger.Warn("feed retry skipped because the retry phase time limit was reached", "source_id", source.ID, "source", source.Name, "initial_error", outcome.err)
	}
}

func (c *Collector) appendRemainingFailures(sources []model.Source, failures map[string]refreshOutcome, result *Result) {
	for _, source := range sources {
		if outcome, failed := failures[source.ID]; failed {
			result.Failures = append(result.Failures, failureFromOutcome(outcome))
		}
	}
}

func failureFromOutcome(outcome refreshOutcome) Failure {
	return Failure{
		SourceID: outcome.source.ID,
		Name:     outcome.source.Name,
		URL:      outcome.source.URL,
		Stage:    outcome.stage,
		Reason:   outcome.err.Error(),
	}
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
