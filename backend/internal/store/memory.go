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
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrBadAction = errors.New("unsupported article action")
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
	s := &Memory{
		articles: make(map[string]model.Article),
		sources:  make(map[string]model.Source),
		tags:     make(map[string]model.Tag),
	}
	s.seed()
	return s
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
		if !matchesArticleState(article.State, filter.State) {
			continue
		}
		article.Body = slices.Clone(article.Body)
		result = append(result, article)
	}
	return result
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

	switch action {
	case model.ActionRead:
		article.State.Read = true
		article.State.Skipped = false
	case model.ActionUnread:
		article.State.Read = false
		article.State.Skipped = false
	case model.ActionSkip:
		article.State.Read = true
		article.State.Skipped = true
	case model.ActionSave:
		article.State.Saved = true
	case model.ActionUnsave:
		article.State.Saved = false
	case model.ActionFavorite:
		article.State.Favorite = true
	case model.ActionUnfavorite:
		article.State.Favorite = false
	case model.ActionDelete:
		article.State.Deleted = true
		article.State.Favorite = false
	case model.ActionRestore:
		article.State.Deleted = false
	default:
		return model.Article{}, ErrBadAction
	}

	s.articles[id] = article
	if source, ok := s.sources[article.SourceID]; ok {
		article.Source = source.Name
		article.TagIDs = slices.Clone(source.TagIDs)
	}
	return article, nil
}

func (s *Memory) ResetSkipped() int {
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
	return count
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
	fetchedAt := time.Now().UTC()
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

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + "-" + hex.EncodeToString(bytes)
}

func (s *Memory) seed() {
	now := time.Now().UTC()
	tags := []model.Tag{
		{ID: "technology", Name: "テクノロジー", CreatedAt: now},
		{ID: "business", Name: "ビジネス", CreatedAt: now},
		{ID: "culture", Name: "カルチャー", CreatedAt: now},
		{ID: "science", Name: "サイエンス", CreatedAt: now},
	}
	for _, tag := range tags {
		s.tags[tag.ID] = tag
		s.tagOrder = append(s.tagOrder, tag.ID)
	}

	sources := []model.Source{
		{ID: "orbit-journal", Name: "Orbit Journal", URL: "https://example.com/orbit/rss.xml", Format: "rss", Enabled: true, TagIDs: []string{"technology", "science"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
		{ID: "business-field", Name: "Business Field", URL: "https://example.com/business/atom.xml", Format: "atom", Enabled: true, TagIDs: []string{"business"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
		{ID: "nook-magazine", Name: "Nook Magazine", URL: "https://example.com/nook/feed.xml", Format: "rss", Enabled: true, TagIDs: []string{"culture"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
		{ID: "scope-science", Name: "Scope Science", URL: "https://example.com/scope/atom.xml", Format: "atom", Enabled: true, TagIDs: []string{"science", "technology"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
		{ID: "common-ledger", Name: "Common Ledger", URL: "https://example.com/ledger/rss.xml", Format: "rss", Enabled: true, TagIDs: []string{"business", "culture"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
		{ID: "open-current", Name: "Open Current", URL: "https://example.com/current/feed.xml", Format: "rss", Enabled: true, TagIDs: []string{"technology", "science"}, ArticleCount: 1, LastFetchedAt: &now, CreatedAt: now},
	}
	for _, source := range sources {
		s.sources[source.ID] = source
		s.sourceOrder = append(s.sourceOrder, source.ID)
	}

	articles := seedArticles()
	for _, article := range articles {
		s.articles[article.ID] = article
		s.articleOrder = append(s.articleOrder, article.ID)
	}
}

func seedArticles() []model.Article {
	return []model.Article{
		{
			ID: "future-interface", SourceID: "orbit-journal", Source: "Orbit Journal", SourceInitials: "OJ",
			PublishedAt: "12分前", ReadTime: 4, Title: "画面のないコンピューターが, 暮らしの輪郭を変えはじめた",
			Summary: "身につけるAIデバイスと音声インターフェース. 次のコンピューティング体験をつくる小さな変化を追う.",
			Body: []string{
				"スマートフォンを取り出さずに情報へ触れる体験が, 少しずつ日常へ入りはじめています. 音声と小型センサーを組み合わせたデバイスは, 必要な瞬間だけ静かに情報を差し出します.",
				"重要なのは画面が消えることではなく, 人と情報の距離が変わることです. 通知を増やすのではなく, 本当に必要な情報を選ぶ設計が求められています.",
			}, VisualLabel: "NEW INTERFACE", VisualTheme: "cobalt",
		},
		{
			ID: "small-city-business", SourceID: "business-field", Source: "Business Field", SourceInitials: "BF",
			PublishedAt: "28分前", ReadTime: 6, Title: "小さな都市から生まれる, 新しい働き方のネットワーク",
			Summary: "場所よりも関係性を選ぶチームが増えている. 地域に根ざしながら世界と働く人々の現在地.",
			Body: []string{
				"都市への集中を前提にしないチームづくりが広がっています. 小さな拠点を行き来しながら, 得意分野の異なる人がプロジェクトごとに集まります.",
				"オンラインだけでは生まれにくい偶然の会話を残しながら, 移動の負担を減らす. その両立が新しい組織設計の焦点です.",
			}, VisualLabel: "LOCAL / GLOBAL", VisualTheme: "coral",
		},
		{
			ID: "night-museum", SourceID: "nook-magazine", Source: "Nook Magazine", SourceInitials: "NM",
			PublishedAt: "1時間前", ReadTime: 3, Title: "夜のミュージアムで出会う, もうひとつの街の表情",
			Summary: "閉館時間を越えて開かれる展示と対話. 静かな夜に文化施設が担う役割を考える.",
			Body: []string{
				"日中とは違う速度で作品と向き合える夜間開館が注目されています. 仕事帰りの人や, 混雑を避けたい人にとって新しい居場所になっています.",
				"展示を見るだけでなく, 小さな対話や音楽が同じ空間に重なることで, 街の文化は少しだけ身近なものになります.",
			}, VisualLabel: "AFTER HOURS", VisualTheme: "violet",
		},
		{
			ID: "deep-sea-sound", SourceID: "scope-science", Source: "Scope Science", SourceInitials: "SS",
			PublishedAt: "2時間前", ReadTime: 5, Title: "深海の音から読み解く, 目に見えない生態系の変化",
			Summary: "水中マイクが捉えた長期データから, 海の季節と生き物の移動が見えてきた.",
			Body: []string{
				"光の届かない海では, 音が環境を知る大切な手がかりになります. 研究チームは長期間の録音から, 生き物の移動や人間活動の影響を分析しています.",
				"同じ場所の音を継続して比べることで, 一度の調査では見えない小さな変化を捉えられるようになりました.",
			}, VisualLabel: "BELOW 2,000M", VisualTheme: "aqua",
		},
		{
			ID: "repair-economy", SourceID: "common-ledger", Source: "Common Ledger", SourceInitials: "CL",
			PublishedAt: "3時間前", ReadTime: 7, Title: "「直して使う」がつくる, 小さくて強い地域経済",
			Summary: "修理する技術と場所を共有するリペアカフェ. モノを長く使うことから生まれる新しい循環.",
			Body: []string{
				"壊れた家電や衣服を持ち寄り, 修理の知識を共有する場所が増えています. 費用を抑えるだけでなく, 技術や道具を地域で受け継ぐ役割もあります.",
				"大量に買い替える経済から, 手入れしながら使う経済へ. 小さな活動が地域の新しいつながりを育てています.",
			}, VisualLabel: "REPAIR / REUSE", VisualTheme: "amber",
		},
		{
			ID: "open-source-garden", SourceID: "open-current", Source: "Open Current", SourceInitials: "OC",
			PublishedAt: "5時間前", ReadTime: 4, Title: "オープンソースで育てる, 都市の小さな菜園",
			Summary: "センサーと共有設計図を使って, 誰でも参加できる都市農園をつくる試み.",
			Body: []string{
				"土の水分や日照を測る小さなセンサーを, 誰でも組み立てられる設計図として公開する活動が始まっています.",
				"技術は収穫量を競うためだけではありません. 初めて参加する人が植物の変化に気づき, 地域の経験を共有するきっかけにもなっています.",
			}, VisualLabel: "OPEN GARDEN", VisualTheme: "forest",
		},
	}
}
