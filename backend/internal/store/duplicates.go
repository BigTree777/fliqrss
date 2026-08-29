package store

import (
	"net"
	"net/url"
	"slices"
	"strings"

	"fliqrss/backend/internal/model"
)

func canonicalArticleURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if port != "" && port != "80" && port != "443" {
		host = net.JoinHostPort(host, port)
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	} else if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || slices.Contains([]string{"fbclid", "gclid", "mc_cid", "mc_eid"}, lower) {
			query.Del(key)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		return host + path + "?" + encoded
	}
	return host + path
}

// ReconcileDuplicates reapplies the saved source priority to every canonical URL.
// Reordering sources intentionally does not call this method; callers decide when
// the new priority should become effective.
func (s *Memory) ReconcileDuplicates() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	affectedURLs := make(map[string]struct{}, len(s.articleURLIndex))
	for canonicalURL := range s.articleURLIndex {
		affectedURLs[canonicalURL] = struct{}{}
	}
	s.reconcileDuplicateURLsLocked(affectedURLs, nil)
	return nil
}

func (s *Memory) reconcileAllDuplicatesWithChanges() []model.Article {
	s.mu.Lock()
	defer s.mu.Unlock()

	affectedURLs := make(map[string]struct{}, len(s.articleURLIndex))
	for canonicalURL := range s.articleURLIndex {
		affectedURLs[canonicalURL] = struct{}{}
	}
	return s.reconcileDuplicateURLsLocked(affectedURLs, nil)
}

func (s *Memory) reconcileDuplicateURLsLocked(urls map[string]struct{}, stateOverrides map[string]model.ArticleState) []model.Article {
	if len(urls) == 0 {
		return nil
	}

	sourcePriority := make(map[string]int, len(s.sourceOrder))
	for priority, sourceID := range s.sourceOrder {
		sourcePriority[sourceID] = priority
	}
	changed := make([]model.Article, 0)
	for canonicalURL := range urls {
		articleIDs := slices.Clone(s.articleURLIndex[canonicalURL])
		if len(articleIDs) == 0 {
			continue
		}
		slices.SortStableFunc(articleIDs, func(firstID, secondID string) int {
			first := s.articles[firstID]
			second := s.articles[secondID]
			if difference := sourcePriority[first.SourceID] - sourcePriority[second.SourceID]; difference != 0 {
				return difference
			}
			return strings.Compare(firstID, secondID)
		})
		distinctSources := make(map[string]struct{})
		for _, articleID := range articleIDs {
			distinctSources[s.articles[articleID].SourceID] = struct{}{}
		}

		var previousState *model.ArticleState
		if override, exists := stateOverrides[canonicalURL]; exists {
			state := override
			previousState = &state
		} else {
			referencedRepresentatives := make(map[string]struct{})
			for _, articleID := range articleIDs {
				if representativeID := s.articles[articleID].DuplicateOfID; representativeID != "" {
					referencedRepresentatives[representativeID] = struct{}{}
				}
			}
			for _, articleID := range articleIDs {
				article := s.articles[articleID]
				_, referenced := referencedRepresentatives[articleID]
				if referenced || article.DuplicateCount > 0 {
					state := article.State
					previousState = &state
					break
				}
			}
			if previousState == nil && len(distinctSources) > 1 {
				state := s.articles[articleIDs[0]].State
				for _, articleID := range articleIDs[1:] {
					candidate := s.articles[articleID].State
					state.Read = state.Read || candidate.Read
					state.Skipped = state.Skipped || candidate.Skipped
					state.Saved = state.Saved || candidate.Saved
					state.Favorite = state.Favorite || candidate.Favorite
					state.Deleted = state.Deleted && candidate.Deleted
				}
				previousState = &state
			}
		}

		representativeID := articleIDs[0]
		representative := s.articles[representativeID]
		if previousState != nil {
			representative.State = *previousState
		}
		representative.DuplicateOfID = ""
		representative.DuplicateReason = ""
		representative.DuplicateCount = 0
		representative.DuplicateSources = nil
		if len(distinctSources) == 1 {
			s.articles[representativeID] = representative
			changed = append(changed, representative)
			for _, articleID := range articleIDs[1:] {
				article := s.articles[articleID]
				article.DuplicateOfID = ""
				article.DuplicateReason = ""
				article.DuplicateCount = 0
				article.DuplicateSources = nil
				s.articles[articleID] = article
				changed = append(changed, article)
			}
			continue
		}
		representative.DuplicateCount = len(articleIDs) - 1

		for _, duplicateID := range articleIDs[1:] {
			duplicate := s.articles[duplicateID]
			duplicate.State = model.ArticleState{}
			duplicate.DuplicateOfID = representativeID
			duplicate.DuplicateReason = "url"
			duplicate.DuplicateCount = 0
			duplicate.DuplicateSources = nil
			s.articles[duplicateID] = duplicate
			changed = append(changed, duplicate)

			if source, exists := s.sources[duplicate.SourceID]; exists && !slices.Contains(representative.DuplicateSources, source.Name) {
				representative.DuplicateSources = append(representative.DuplicateSources, source.Name)
			}
		}
		s.articles[representativeID] = representative
		changed = append(changed, representative)
	}

	return changed
}

func (s *Memory) representativeStateForURLLocked(canonicalURL string) (model.ArticleState, bool) {
	for _, articleID := range s.articleURLIndex[canonicalURL] {
		article, exists := s.articles[articleID]
		if exists && article.DuplicateOfID == "" {
			return article.State, true
		}
	}
	return model.ArticleState{}, false
}

func (s *Memory) addArticleURLIndexLocked(canonicalURL, articleID string) {
	if canonicalURL == "" || slices.Contains(s.articleURLIndex[canonicalURL], articleID) {
		return
	}
	s.articleURLIndex[canonicalURL] = append(s.articleURLIndex[canonicalURL], articleID)
}

func (s *Memory) removeArticleURLIndexLocked(canonicalURL, articleID string) {
	if canonicalURL == "" {
		return
	}
	remaining := slices.DeleteFunc(s.articleURLIndex[canonicalURL], func(candidate string) bool {
		return candidate == articleID
	})
	if len(remaining) == 0 {
		delete(s.articleURLIndex, canonicalURL)
		return
	}
	s.articleURLIndex[canonicalURL] = remaining
}

func (s *Memory) addSourceArticleIndexLocked(sourceID, articleID string) {
	if sourceID == "" || slices.Contains(s.sourceArticleIndex[sourceID], articleID) {
		return
	}
	s.sourceArticleIndex[sourceID] = append(s.sourceArticleIndex[sourceID], articleID)
}

func (s *Memory) removeSourceArticleIndexLocked(sourceID, articleID string) {
	if sourceID == "" {
		return
	}
	remaining := slices.DeleteFunc(s.sourceArticleIndex[sourceID], func(candidate string) bool {
		return candidate == articleID
	})
	if len(remaining) == 0 {
		delete(s.sourceArticleIndex, sourceID)
		return
	}
	s.sourceArticleIndex[sourceID] = remaining
}

func uniqueArticleChanges(changes ...[]model.Article) []model.Article {
	byID := make(map[string]model.Article)
	order := make([]string, 0)
	for _, group := range changes {
		for _, article := range group {
			if _, exists := byID[article.ID]; !exists {
				order = append(order, article.ID)
			}
			byID[article.ID] = article
		}
	}
	result := make([]model.Article, 0, len(order))
	for _, articleID := range order {
		result = append(result, byID[articleID])
	}
	return result
}
