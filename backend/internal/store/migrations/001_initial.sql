CREATE TABLE IF NOT EXISTS tags (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    sort_order BIGSERIAL NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS tags_name_unique ON tags (LOWER(name));

CREATE TABLE IF NOT EXISTS sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    format TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    article_count INTEGER NOT NULL DEFAULT 0,
    last_fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    sort_order BIGSERIAL NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS sources_url_unique ON sources (LOWER(url));

CREATE TABLE IF NOT EXISTS source_tags (
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (source_id, tag_id)
);

CREATE TABLE IF NOT EXISTS articles (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    source_initials TEXT NOT NULL DEFAULT '',
    published_at TEXT NOT NULL DEFAULT '',
    read_time INTEGER NOT NULL DEFAULT 1,
    title TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    body JSONB NOT NULL DEFAULT '[]'::jsonb,
    visual_label TEXT NOT NULL DEFAULT '',
    visual_theme TEXT NOT NULL DEFAULT '',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    is_skipped BOOLEAN NOT NULL DEFAULT FALSE,
    is_saved BOOLEAN NOT NULL DEFAULT FALSE,
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order BIGSERIAL NOT NULL
);

CREATE INDEX IF NOT EXISTS articles_source_id_index ON articles (source_id);
