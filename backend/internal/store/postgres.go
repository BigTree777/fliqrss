package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"fliqrss/backend/internal/model"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/001_initial.sql
var initialMigration string

const databaseOperationTimeout = 10 * time.Second

type PostgreSQL struct {
	db     *sql.DB
	memory *Memory
	mu     sync.Mutex
}

func OpenPostgreSQL(ctx context.Context, databaseURL string) (*PostgreSQL, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	if err := migratePostgreSQL(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	store := &PostgreSQL{db: db, memory: NewMemory()}
	if err := store.load(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("load PostgreSQL: %w", err)
	}
	return store, nil
}

func migratePostgreSQL(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range strings.Split(initialMigration, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PostgreSQL) Close() error {
	return s.db.Close()
}

func (s *PostgreSQL) ListArticles(filter model.ArticleFilter) []model.Article {
	return s.memory.ListArticles(filter)
}

func (s *PostgreSQL) ListArticlePage(filter model.ArticleFilter, cursor string, limit int) (model.ArticlePage, error) {
	return s.memory.ListArticlePage(filter, cursor, limit)
}

func (s *PostgreSQL) ArticleStats() model.ArticleStats {
	return s.memory.ArticleStats()
}

func (s *PostgreSQL) GetArticle(id string) (model.Article, error) {
	return s.memory.GetArticle(id)
}

func (s *PostgreSQL) ListSources() []model.Source {
	return s.memory.ListSources()
}

func (s *PostgreSQL) GetSource(id string) (model.Source, error) {
	return s.memory.GetSource(id)
}

func (s *PostgreSQL) HasSourceURL(rawURL string) bool {
	return s.memory.HasSourceURL(rawURL)
}

func (s *PostgreSQL) ListTags() []model.Tag {
	return s.memory.ListTags()
}

func (s *PostgreSQL) ApplyArticleAction(id string, action model.ArticleAction) (model.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	article, err := s.memory.GetArticle(id)
	if err != nil {
		return model.Article{}, err
	}
	state, err := applyArticleAction(article.State, action)
	if err != nil {
		return model.Article{}, err
	}
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `UPDATE articles SET is_read=$2, is_skipped=$3, is_saved=$4, is_favorite=$5, is_deleted=$6 WHERE id=$1`,
		id, state.Read, state.Skipped, state.Saved, state.Favorite, state.Deleted)
	if err != nil {
		return model.Article{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.Article{}, ErrNotFound
	}
	return s.memory.ApplyArticleAction(id, action)
}

func (s *PostgreSQL) ResetSkipped() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `UPDATE articles SET is_read=FALSE, is_skipped=FALSE WHERE is_skipped=TRUE`)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	_, _ = s.memory.ResetSkipped()
	return int(count), nil
}

func (s *PostgreSQL) CreateSource(name, rawURL, format string) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memory.HasSourceURL(rawURL) {
		return model.Source{}, ErrConflict
	}
	source := model.Source{ID: newID("src"), Name: name, URL: rawURL, Format: format, Enabled: true, TagIDs: []string{}, CreatedAt: time.Now().UTC()}
	ctx, cancel := s.operationContext()
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sources (id,name,url,format,enabled,article_count,last_fetched_at,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		source.ID, source.Name, source.URL, source.Format, source.Enabled, source.ArticleCount, source.LastFetchedAt, source.CreatedAt)
	if isUniqueViolation(err) {
		return model.Source{}, ErrConflict
	}
	if err != nil {
		return model.Source{}, err
	}
	s.memory.insertSource(source)
	return source, nil
}

func (s *PostgreSQL) UpsertArticles(sourceID, format string, articles []model.Article) (model.Source, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.memory.GetSource(sourceID); err != nil {
		return model.Source{}, 0, err
	}
	ctx, cancel := s.operationContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Source{}, 0, err
	}
	defer tx.Rollback()
	for _, article := range articles {
		body, err := json.Marshal(article.Body)
		if err != nil {
			return model.Source{}, 0, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO articles
            (id,source_id,source_initials,published_at,read_time,title,url,summary,body,visual_label,visual_theme,is_read,is_skipped,is_saved,is_favorite,is_deleted)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16)
            ON CONFLICT (id) DO UPDATE SET source_id=EXCLUDED.source_id,source_initials=EXCLUDED.source_initials,published_at=EXCLUDED.published_at,
            read_time=EXCLUDED.read_time,title=EXCLUDED.title,url=EXCLUDED.url,summary=EXCLUDED.summary,body=EXCLUDED.body,
            visual_label=EXCLUDED.visual_label,visual_theme=EXCLUDED.visual_theme`,
			article.ID, sourceID, article.SourceInitials, article.PublishedAt, article.ReadTime, article.Title, article.URL, article.Summary,
			string(body), article.VisualLabel, article.VisualTheme, article.State.Read, article.State.Skipped, article.State.Saved, article.State.Favorite, article.State.Deleted)
		if err != nil {
			return model.Source{}, 0, err
		}
	}
	fetchedAt := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE sources SET format=$2,last_fetched_at=$3,article_count=(SELECT COUNT(*) FROM articles WHERE source_id=$1) WHERE id=$1`, sourceID, format, fetchedAt); err != nil {
		return model.Source{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return model.Source{}, 0, err
	}
	return s.memory.upsertArticlesAt(sourceID, format, articles, fetchedAt)
}

func (s *PostgreSQL) UpdateSource(id string, name *string, enabled *bool) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.memory.GetSource(id)
	if err != nil {
		return model.Source{}, err
	}
	if name != nil {
		current.Name = *name
	}
	if enabled != nil {
		current.Enabled = *enabled
	}
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `UPDATE sources SET name=$2,enabled=$3 WHERE id=$1`, id, current.Name, current.Enabled)
	if err != nil {
		return model.Source{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.Source{}, ErrNotFound
	}
	return s.memory.UpdateSource(id, name, enabled)
}

func (s *PostgreSQL) DeleteSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return s.memory.DeleteSource(id)
}

func (s *PostgreSQL) SetSourceTags(id string, tagIDs []string) (model.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.memory.GetSource(id); err != nil {
		return model.Source{}, err
	}
	for _, tagID := range tagIDs {
		if !s.memory.hasTag(tagID) {
			return model.Source{}, ErrNotFound
		}
	}
	tagIDs = uniqueStrings(tagIDs)
	ctx, cancel := s.operationContext()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Source{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_tags WHERE source_id=$1`, id); err != nil {
		return model.Source{}, err
	}
	for position, tagID := range tagIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_tags (source_id,tag_id,position) VALUES ($1,$2,$3)`, id, tagID, position); err != nil {
			return model.Source{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Source{}, err
	}
	return s.memory.SetSourceTags(id, tagIDs)
}

func (s *PostgreSQL) CreateTag(name string) (model.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.memory.findTagByName(name); ok {
		return model.Tag{}, ErrConflict
	}
	tag := model.Tag{ID: newID("tag"), Name: name, CreatedAt: time.Now().UTC()}
	ctx, cancel := s.operationContext()
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO tags (id,name,created_at) VALUES ($1,$2,$3)`, tag.ID, tag.Name, tag.CreatedAt)
	if isUniqueViolation(err) {
		return model.Tag{}, ErrConflict
	}
	if err != nil {
		return model.Tag{}, err
	}
	s.memory.insertTag(tag)
	return tag, nil
}

func (s *PostgreSQL) FindOrCreateTag(name string) (model.Tag, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tag, ok := s.memory.findTagByName(name); ok {
		return tag, false, nil
	}
	tag := model.Tag{ID: newID("tag"), Name: name, CreatedAt: time.Now().UTC()}
	ctx, cancel := s.operationContext()
	defer cancel()
	_, err := s.db.ExecContext(ctx, `INSERT INTO tags (id,name,created_at) VALUES ($1,$2,$3)`, tag.ID, tag.Name, tag.CreatedAt)
	if isUniqueViolation(err) {
		if existing, ok := s.memory.findTagByName(name); ok {
			return existing, false, nil
		}
	}
	if err != nil {
		return model.Tag{}, false, err
	}
	s.memory.insertTag(tag)
	return tag, true, nil
}

func (s *PostgreSQL) UpdateTag(id, name string) (model.Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `UPDATE tags SET name=$2 WHERE id=$1`, id, name)
	if isUniqueViolation(err) {
		return model.Tag{}, ErrConflict
	}
	if err != nil {
		return model.Tag{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return model.Tag{}, ErrNotFound
	}
	return s.memory.UpdateTag(id, name)
}

func (s *PostgreSQL) DeleteTag(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := s.operationContext()
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return s.memory.DeleteTag(id)
}

func (s *PostgreSQL) load(ctx context.Context) error {
	tags, err := s.loadTags(ctx)
	if err != nil {
		return err
	}
	sources, err := s.loadSources(ctx)
	if err != nil {
		return err
	}
	articles, err := s.loadArticles(ctx)
	if err != nil {
		return err
	}
	s.memory.replace(tags, sources, articles)
	return nil
}

func (s *PostgreSQL) loadTags(ctx context.Context) ([]model.Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,created_at FROM tags ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Tag
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}

func (s *PostgreSQL) loadSources(ctx context.Context) ([]model.Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,url,format,enabled,article_count,last_fetched_at,created_at FROM sources ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Source
	byID := make(map[string]int)
	for rows.Next() {
		var source model.Source
		var fetchedAt sql.NullTime
		if err := rows.Scan(&source.ID, &source.Name, &source.URL, &source.Format, &source.Enabled, &source.ArticleCount, &fetchedAt, &source.CreatedAt); err != nil {
			return nil, err
		}
		if fetchedAt.Valid {
			source.LastFetchedAt = &fetchedAt.Time
		}
		source.TagIDs = []string{}
		byID[source.ID] = len(result)
		result = append(result, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tagRows, err := s.db.QueryContext(ctx, `SELECT source_id,tag_id FROM source_tags ORDER BY source_id,position`)
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var sourceID, tagID string
		if err := tagRows.Scan(&sourceID, &tagID); err != nil {
			return nil, err
		}
		if index, ok := byID[sourceID]; ok {
			result[index].TagIDs = append(result[index].TagIDs, tagID)
		}
	}
	return result, tagRows.Err()
}

func (s *PostgreSQL) loadArticles(ctx context.Context) ([]model.Article, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,source_id,source_initials,published_at,read_time,title,url,summary,body,visual_label,visual_theme,is_read,is_skipped,is_saved,is_favorite,is_deleted FROM articles ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []model.Article
	for rows.Next() {
		var article model.Article
		var body []byte
		if err := rows.Scan(&article.ID, &article.SourceID, &article.SourceInitials, &article.PublishedAt, &article.ReadTime, &article.Title,
			&article.URL, &article.Summary, &body, &article.VisualLabel, &article.VisualTheme, &article.State.Read, &article.State.Skipped,
			&article.State.Saved, &article.State.Favorite, &article.State.Deleted); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &article.Body); err != nil {
			return nil, err
		}
		result = append(result, article)
	}
	return result, rows.Err()
}

func (s *PostgreSQL) operationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), databaseOperationTimeout)
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

var _ Repository = (*PostgreSQL)(nil)
