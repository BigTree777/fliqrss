package model

import "time"

type ArticleState struct {
	Read     bool `json:"read"`
	Skipped  bool `json:"skipped"`
	Saved    bool `json:"saved"`
	Favorite bool `json:"favorite"`
	Deleted  bool `json:"deleted"`
}

type Article struct {
	ID               string       `json:"id"`
	SourceID         string       `json:"sourceId"`
	Source           string       `json:"source"`
	SourceInitials   string       `json:"sourceInitials"`
	PublishedAt      string       `json:"publishedAt"`
	ReadTime         int          `json:"readTime"`
	Title            string       `json:"title"`
	URL              string       `json:"url,omitempty"`
	Summary          string       `json:"summary"`
	Body             []string     `json:"body"`
	VisualLabel      string       `json:"visualLabel"`
	VisualTheme      string       `json:"visualTheme"`
	TagIDs           []string     `json:"tagIds"`
	State            ArticleState `json:"state"`
	CanonicalURL     string       `json:"-"`
	DuplicateOfID    string       `json:"duplicateOfId,omitempty"`
	DuplicateReason  string       `json:"duplicateReason,omitempty"`
	DuplicateCount   int          `json:"duplicateCount,omitempty"`
	DuplicateSources []string     `json:"duplicateSources,omitempty"`
}

type Source struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Format        string     `json:"format"`
	Enabled       bool       `json:"enabled"`
	TagIDs        []string   `json:"tagIds"`
	ArticleCount  int        `json:"articleCount"`
	LastFetchedAt *time.Time `json:"lastFetchedAt"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type ArticleFilter struct {
	SourceID string
	TagID    string
	Untagged bool
	State    string
}

type ArticlePage struct {
	Items      []Article `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
	Total      int       `json:"total"`
}

type ArticleStats struct {
	Feed             int            `json:"feed"`
	Favorite         int            `json:"favorite"`
	Saved            int            `json:"saved"`
	Deleted          int            `json:"deleted"`
	Skipped          int            `json:"skipped"`
	UntaggedFeed     int            `json:"untaggedFeed"`
	SourceFeedCounts map[string]int `json:"sourceFeedCounts"`
	TagFeedCounts    map[string]int `json:"tagFeedCounts"`
}

type ArticleAction string

const (
	ActionRead       ArticleAction = "read"
	ActionUnread     ArticleAction = "unread"
	ActionSkip       ArticleAction = "skip"
	ActionSave       ArticleAction = "save"
	ActionUnsave     ArticleAction = "unsave"
	ActionFavorite   ArticleAction = "favorite"
	ActionUnfavorite ArticleAction = "unfavorite"
	ActionDelete     ArticleAction = "delete"
	ActionRestore    ArticleAction = "restore"
)
