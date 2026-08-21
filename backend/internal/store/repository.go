package store

import "fliqrss/backend/internal/model"

type Repository interface {
	ListArticles(model.ArticleFilter) []model.Article
	GetArticle(string) (model.Article, error)
	ApplyArticleAction(string, model.ArticleAction) (model.Article, error)
	ResetSkipped() (int, error)

	ListSources() []model.Source
	GetSource(string) (model.Source, error)
	HasSourceURL(string) bool
	CreateSource(string, string, string) (model.Source, error)
	UpsertArticles(string, string, []model.Article) (model.Source, int, error)
	UpdateSource(string, *string, *bool) (model.Source, error)
	DeleteSource(string) error
	SetSourceTags(string, []string) (model.Source, error)

	ListTags() []model.Tag
	CreateTag(string) (model.Tag, error)
	FindOrCreateTag(string) (model.Tag, bool, error)
	UpdateTag(string, string) (model.Tag, error)
	DeleteTag(string) error
}
