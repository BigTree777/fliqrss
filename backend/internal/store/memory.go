package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"fliqrss/backend/internal/model"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrBadAction     = errors.New("unsupported article action")
	ErrInvalidCursor = errors.New("invalid cursor")
)

type Memory struct {
	mu           sync.RWMutex
	articles     map[string]model.Article
	articleOrder []string
	sources      map[string]model.Source
	sourceOrder  []string
	tags         map[string]model.Tag
	tagOrder     []string
}

func NewMemory() *Memory {
	return &Memory{
		articles: make(map[string]model.Article),
		sources:  make(map[string]model.Source),
		tags:     make(map[string]model.Tag),
	}
}

func (s *Memory) ListArticles(filter model.ArticleFilter) []model.Article {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Article, 0, len(s.articleOrder))
	for _, id := range s.articleOrder {
		article := s.articles[id]
		source, sourceExists := s.sources[article.SourceID]
		if sourceExists {
			article.Source = source.Name
			article.TagIDs = slices.Clone(source.TagIDs)
		}
		if filter.SourceID != "" && article.SourceID != filter.SourceID {
			continue
		}
		if filter.TagID != "" && !slices.Contains(article.TagIDs, filter.TagID) {
			continue
		}
		if filter.Untagged && len(article.TagIDs) != 0 {
			continue
		}
		if !matchesArticleState(article.State, filter.State) {
			continue
		}
		article.Body = slices.Clone(article.Body)
		result = append(result, article)
	}
	return result
}

func (s *Memory) ListArticlePage(filter model.ArticleFilter, cursor string, limit int) (model.ArticlePage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit < 1 {
		limit = 1
	}
	start := 0
	if cursor != "" {
		index := slices.Index(s.articleOrder, cursor)
		if index < 0 {
			return model.ArticlePage{}, ErrInvalidCursor
		}
		start = index + 1
	}

	matching := make([]model.Article, 0, limit+1)
	total := 0
	for index, id := range s.articleOrder {
		article := s.articleForRead(id)
		if !matchesArticleFilter(article, filter) {
			continue
		}
		total++
		if index >= start && len(matching) <= limit {
			matching = append(matching, article)
		}
	}

	page := model.ArticlePage{Items: matching, Total: total}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (s *Memory) ArticleStats() model.ArticleStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := model.ArticleStats{
		SourceFeedCounts: make(map[string]int),
		TagFeedCounts:    make(map[string]int),
	}
	for _, id := range s.articleOrder {
		article := s.articleForRead(id)
		if matchesArticleState(article.State, "feed") {
			stats.Feed++
			stats.SourceFeedCounts[article.SourceID]++
			if len(article.TagIDs) == 0 {
				stats.UntaggedFeed++
			}
			for _, tagID := range article.TagIDs {
				stats.TagFeedCounts[tagID]++
			}
		}
		if article.State.Favorite {
			stats.Favorite++
		}
		if article.State.Saved {
			stats.Saved++
		}
		if article.State.Deleted {
			stats.Deleted++
		}
		if article.State.Skipped {
			stats.Skipped++
		}
	}
	return stats
}

func (s *Memory) articleForRead(id string) model.Article {
	article := s.articles[id]
	if source, ok := s.sources[article.SourceID]; ok {
		article.Source = source.Name
		article.TagIDs = slices.Clone(source.TagIDs)
	}
	article.Body = slices.Clone(article.Body)
	return article
}

func matchesArticleFilter(article model.Article, filter model.ArticleFilter) bool {
	if filter.SourceID != "" && article.SourceID != filter.SourceID {
		return false
	}
	if filter.TagID != "" && !slices.Contains(article.TagIDs, filter.TagID) {
		return false
	}
	if filter.Untagged && len(article.TagIDs) != 0 {
		return false
	}
	return matchesArticleState(article.State, filter.State)
}

func matchesArticleState(state model.ArticleState, requested string) bool {
	switch requested {
	case "", "feed":
		return !state.Read && !state.Saved && !state.Deleted
	case "all":
		return true
	case "favorite":
		return state.Favorite
	case "saved":
		return state.Saved
	case "deleted":
		return state.Deleted
	case "read":
		return state.Read
	default:
		return false
	}
}

func (s *Memory) GetArticle(id string) (model.Article, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	article, ok := s.articles[id]
	if !ok {
		return model.Article{}, ErrNotFound
	}
	if source, ok := s.sources[article.SourceID]; ok {
		article.Source = source.Name
		article.TagIDs = slices.Clone(source.TagIDs)
	}
	article.Body = slices.Clone(article.Body)
	return article, nil
}

func (s *Memory) ApplyArticleAction(id string, action model.ArticleAction) (model.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	article, ok := s.articles[id]
	if !ok {
		return model.Article{}, ErrNotFound
	}

	state, err := applyArticleAction(article.State, action)
	if err != nil {
		return model.Article{}, err
	}
	article.State = state

	s.articles[id] = article
	if source, ok := s.sources[article.SourceID]; ok {
		article.Source = source.Name
		article.TagIDs = slices.Clone(source.TagIDs)
	}
	return article, nil
}

func applyArticleAction(state model.ArticleState, action model.ArticleAction) (model.ArticleState, error) {
	switch action {
	case model.ActionRead:
		state.Read = true
		state.Skipped = false
	case model.ActionUnread:
		state.Read = false
		state.Skipped = false
	case model.ActionSkip:
		state.Read = true
		state.Skipped = true
	case model.ActionSave:
		state.Saved = true
	case model.ActionUnsave:
		state.Saved = false
	case model.ActionFavorite:
		state.Favorite = true
	case model.ActionUnfavorite:
		state.Favorite = false
	case model.ActionDelete:
		state.Deleted = true
		state.Favorite = false
	case model.ActionRestore:
		state.Deleted = false
	default:
		return model.ArticleState{}, ErrBadAction
	}
	return state, nil
}

func (s *Memory) ResetSkipped() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for id, article := range s.articles {
		if !article.State.Skipped {
			continue
		}
		article.State.Read = false
		article.State.Skipped = false
		s.articles[id] = article
		count++
	}
	return count, nil
}

func (s *Memory) ListSources() []model.Source {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Source, 0, len(s.sourceOrder))
	for _, id := range s.sourceOrder {
		source := s.sources[id]
		source.TagIDs = slices.Clone(source.TagIDs)
		result = append(result, source)
	}
	return result
}

func (s *Memory) GetSource(id string) (model.Source, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	source, ok := s.sources[id]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	source.TagIDs = slices.Clone(source.TagIDs)
	return source, nil
}

func (s *Memory) HasSourceURL(rawURL string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, source := range s.sources {
		if strings.EqualFold(source.URL, rawURL) {
			return true
		}
	}
	return false
}

func (s *Memory) CreateSource(name, rawURL, format string) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, source := range s.sources {
		if strings.EqualFold(source.URL, rawURL) {
			return model.Source{}, ErrConflict
		}
	}

	source := model.Source{
		ID:        newID("src"),
		Name:      name,
		URL:       rawURL,
		Format:    format,
		Enabled:   true,
		TagIDs:    []string{},
		CreatedAt: time.Now().UTC(),
	}
	s.sources[source.ID] = source
	s.sourceOrder = append(s.sourceOrder, source.ID)
	return source, nil
}

func (s *Memory) UpsertArticles(sourceID, format string, articles []model.Article) (model.Source, int, error) {
	return s.upsertArticlesAt(sourceID, format, articles, time.Now().UTC())
}

func (s *Memory) upsertArticlesAt(sourceID, format string, articles []model.Article, fetchedAt time.Time) (model.Source, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sources[sourceID]
	if !ok {
		return model.Source{}, 0, ErrNotFound
	}
	added := 0
	for _, article := range articles {
		article.SourceID = sourceID
		article.Source = source.Name
		if existing, exists := s.articles[article.ID]; exists {
			article.State = existing.State
			s.articles[article.ID] = article
			continue
		}
		s.articles[article.ID] = article
		s.articleOrder = append(s.articleOrder, article.ID)
		added++
	}
	source.Format = format
	source.LastFetchedAt = &fetchedAt
	source.ArticleCount = 0
	for _, article := range s.articles {
		if article.SourceID == sourceID {
			source.ArticleCount++
		}
	}
	s.sources[sourceID] = source
	return source, added, nil
}

func (s *Memory) UpdateSource(id string, name *string, enabled *bool) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sources[id]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	if name != nil {
		source.Name = *name
	}
	if enabled != nil {
		source.Enabled = *enabled
	}
	s.sources[id] = source
	return source, nil
}

func (s *Memory) DeleteSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sources[id]; !ok {
		return ErrNotFound
	}
	delete(s.sources, id)
	s.sourceOrder = slices.DeleteFunc(s.sourceOrder, func(candidate string) bool { return candidate == id })
	for articleID, article := range s.articles {
		if article.SourceID == id {
			delete(s.articles, articleID)
		}
	}
	s.articleOrder = slices.DeleteFunc(s.articleOrder, func(articleID string) bool {
		_, exists := s.articles[articleID]
		return !exists
	})
	return nil
}

func (s *Memory) SetSourceTags(id string, tagIDs []string) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, ok := s.sources[id]
	if !ok {
		return model.Source{}, ErrNotFound
	}
	unique := make([]string, 0, len(tagIDs))
	seen := make(map[string]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		if _, ok := s.tags[tagID]; !ok {
			return model.Source{}, ErrNotFound
		}
		if _, exists := seen[tagID]; exists {
			continue
		}
		seen[tagID] = struct{}{}
		unique = append(unique, tagID)
	}
	source.TagIDs = unique
	s.sources[id] = source
	return source, nil
}

func (s *Memory) ListTags() []model.Tag {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]model.Tag, 0, len(s.tagOrder))
	for _, id := range s.tagOrder {
		result = append(result, s.tags[id])
	}
	return result
}

func (s *Memory) CreateTag(name string) (model.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tag := range s.tags {
		if strings.EqualFold(tag.Name, name) {
			return model.Tag{}, ErrConflict
		}
	}
	tag := model.Tag{ID: newID("tag"), Name: name, CreatedAt: time.Now().UTC()}
	s.tags[tag.ID] = tag
	s.tagOrder = append(s.tagOrder, tag.ID)
	return tag, nil
}

func (s *Memory) FindOrCreateTag(name string) (model.Tag, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tag := range s.tags {
		if strings.EqualFold(tag.Name, name) {
			return tag, false, nil
		}
	}
	tag := model.Tag{ID: newID("tag"), Name: name, CreatedAt: time.Now().UTC()}
	s.tags[tag.ID] = tag
	s.tagOrder = append(s.tagOrder, tag.ID)
	return tag, true, nil
}

func (s *Memory) UpdateTag(id, name string) (model.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tag, ok := s.tags[id]
	if !ok {
		return model.Tag{}, ErrNotFound
	}
	for candidateID, candidate := range s.tags {
		if candidateID != id && strings.EqualFold(candidate.Name, name) {
			return model.Tag{}, ErrConflict
		}
	}
	tag.Name = name
	s.tags[id] = tag
	return tag, nil
}

func (s *Memory) DeleteTag(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tags[id]; !ok {
		return ErrNotFound
	}
	delete(s.tags, id)
	s.tagOrder = slices.DeleteFunc(s.tagOrder, func(candidate string) bool { return candidate == id })
	for sourceID, source := range s.sources {
		source.TagIDs = slices.DeleteFunc(source.TagIDs, func(candidate string) bool { return candidate == id })
		s.sources[sourceID] = source
	}
	return nil
}

func (s *Memory) insertSource(source model.Source) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source.TagIDs = slices.Clone(source.TagIDs)
	if _, exists := s.sources[source.ID]; !exists {
		s.sourceOrder = append(s.sourceOrder, source.ID)
	}
	s.sources[source.ID] = source
}

func (s *Memory) insertTag(tag model.Tag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tags[tag.ID]; !exists {
		s.tagOrder = append(s.tagOrder, tag.ID)
	}
	s.tags[tag.ID] = tag
}

func (s *Memory) hasTag(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.tags[id]
	return ok
}

func (s *Memory) findTagByName(name string) (model.Tag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, tag := range s.tags {
		if strings.EqualFold(tag.Name, name) {
			return tag, true
		}
	}
	return model.Tag{}, false
}

func (s *Memory) replace(tags []model.Tag, sources []model.Source, articles []model.Article) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.articles = make(map[string]model.Article, len(articles))
	s.articleOrder = make([]string, 0, len(articles))
	for _, article := range articles {
		article.Body = slices.Clone(article.Body)
		s.articles[article.ID] = article
		s.articleOrder = append(s.articleOrder, article.ID)
	}

	s.sources = make(map[string]model.Source, len(sources))
	s.sourceOrder = make([]string, 0, len(sources))
	for _, source := range sources {
		source.TagIDs = slices.Clone(source.TagIDs)
		s.sources[source.ID] = source
		s.sourceOrder = append(s.sourceOrder, source.ID)
	}

	s.tags = make(map[string]model.Tag, len(tags))
	s.tagOrder = make([]string, 0, len(tags))
	for _, tag := range tags {
		s.tags[tag.ID] = tag
		s.tagOrder = append(s.tagOrder, tag.ID)
	}
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(bytes)
}

var _ Repository = (*Memory)(nil)
