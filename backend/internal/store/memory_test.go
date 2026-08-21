package store

import (
	"testing"

	"fliqrss/backend/internal/model"
)

func TestNewMemoryStartsEmpty(t *testing.T) {
	memory := NewMemory()
	if articles := memory.ListArticles(model.ArticleFilter{State: "all"}); len(articles) != 0 {
		t.Fatalf("initial articles = %d, want 0", len(articles))
	}
	if sources := memory.ListSources(); len(sources) != 0 {
		t.Fatalf("initial sources = %d, want 0", len(sources))
	}
	if tags := memory.ListTags(); len(tags) != 0 {
		t.Fatalf("initial tags = %d, want 0", len(tags))
	}
}
