package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/model"
	"fliqrss/backend/internal/store"
)

func TestArticleWorkflow(t *testing.T) {
	server := NewServer(store.NewMemory(), "http://localhost:5173")

	response := performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list feed status = %d, want %d", response.Code, http.StatusOK)
	}
	articles := decodeData[[]model.Article](t, response)
	if len(articles) != 6 {
		t.Fatalf("initial feed length = %d, want 6", len(articles))
	}

	response = performRequest(t, server.Handler(), http.MethodPatch, "/api/v1/articles/future-interface/state", map[string]string{"action": "skip"})
	if response.Code != http.StatusOK {
		t.Fatalf("skip status = %d, want %d", response.Code, http.StatusOK)
	}
	article := decodeData[model.Article](t, response)
	if !article.State.Read || !article.State.Skipped {
		t.Fatalf("skip state = %+v, want read and skipped", article.State)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles", nil)
	articles = decodeData[[]model.Article](t, response)
	if len(articles) != 5 {
		t.Fatalf("feed length after skip = %d, want 5", len(articles))
	}

	response = performRequest(t, server.Handler(), http.MethodPost, "/api/v1/articles/reset-skipped", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("reset skipped status = %d, want %d", response.Code, http.StatusOK)
	}
	result := decodeData[map[string]int](t, response)
	if result["restored"] != 1 {
		t.Fatalf("restored = %d, want 1", result["restored"])
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles", nil)
	articles = decodeData[[]model.Article](t, response)
	if len(articles) != 6 {
		t.Fatalf("feed length after reset = %d, want 6", len(articles))
	}
}

func TestArticleFiltersAndActions(t *testing.T) {
	server := NewServer(store.NewMemory(), "")

	response := performRequest(t, server.Handler(), http.MethodPatch, "/api/v1/articles/future-interface/state", map[string]string{"action": "save"})
	if response.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d", response.Code, http.StatusOK)
	}
	response = performRequest(t, server.Handler(), http.MethodPatch, "/api/v1/articles/future-interface/state", map[string]string{"action": "favorite"})
	article := decodeData[model.Article](t, response)
	if !article.State.Saved || !article.State.Favorite {
		t.Fatalf("saved favorite state = %+v", article.State)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles?state=saved", nil)
	articles := decodeData[[]model.Article](t, response)
	if len(articles) != 1 || articles[0].ID != "future-interface" {
		t.Fatalf("saved articles = %+v, want future-interface", articles)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles?tagId=business&state=all", nil)
	articles = decodeData[[]model.Article](t, response)
	if len(articles) != 2 {
		t.Fatalf("business article count = %d, want 2", len(articles))
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles?state=unknown", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid filter status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestSourceAndTagWorkflow(t *testing.T) {
	server := NewServer(store.NewMemory(), "")

	response := performRequest(t, server.Handler(), http.MethodPost, "/api/v1/tags", map[string]string{"name": "デザイン"})
	if response.Code != http.StatusCreated {
		t.Fatalf("create tag status = %d, want %d", response.Code, http.StatusCreated)
	}
	tag := decodeData[model.Tag](t, response)

	response = performRequest(t, server.Handler(), http.MethodPut, "/api/v1/sources/orbit-journal/tags", map[string][]string{"tagIds": {tag.ID}})
	if response.Code != http.StatusOK {
		t.Fatalf("set source tags status = %d, want %d", response.Code, http.StatusOK)
	}
	source := decodeData[model.Source](t, response)
	if len(source.TagIDs) != 1 || source.TagIDs[0] != tag.ID {
		t.Fatalf("source tags = %v, want %s", source.TagIDs, tag.ID)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles?tagId="+tag.ID+"&state=all", nil)
	articles := decodeData[[]model.Article](t, response)
	if len(articles) != 1 || articles[0].SourceID != source.ID {
		t.Fatalf("tagged articles = %+v", articles)
	}

	response = performRequest(t, server.Handler(), http.MethodDelete, "/api/v1/tags/"+tag.ID, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete tag status = %d, want %d", response.Code, http.StatusNoContent)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/sources", nil)
	sources := decodeData[[]model.Source](t, response)
	for _, candidate := range sources {
		if candidate.ID == "orbit-journal" && len(candidate.TagIDs) != 0 {
			t.Fatalf("deleted tag remains assigned: %v", candidate.TagIDs)
		}
	}
}

func TestDeletingSourceRemovesItsArticles(t *testing.T) {
	server := NewServer(store.NewMemory(), "")

	response := performRequest(t, server.Handler(), http.MethodDelete, "/api/v1/sources/orbit-journal", nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete source status = %d, want %d", response.Code, http.StatusNoContent)
	}

	response = performRequest(t, server.Handler(), http.MethodGet, "/api/v1/articles?state=all", nil)
	articles := decodeData[[]model.Article](t, response)
	if len(articles) != 5 {
		t.Fatalf("article count after source deletion = %d, want 5", len(articles))
	}
	for _, article := range articles {
		if article.SourceID == "orbit-journal" {
			t.Fatalf("article from deleted source remains: %s", article.ID)
		}
	}
}

func TestCreateSourceValidationAndConflict(t *testing.T) {
	loader := staticFeedLoader{document: feed.Document{
		Format: "rss",
		Title:  "New Source",
		Entries: []feed.Entry{{
			ID: "entry-1", Title: "Imported article", Link: "https://news.example.test/articles/1",
			PublishedAt: "2026-08-21T00:00:00Z", Summary: "Summary", Content: "Full content",
		}},
	}}
	server := NewServerWithFeedLoader(store.NewMemory(), "", loader)

	response := performRequest(t, server.Handler(), http.MethodPost, "/api/v1/sources", map[string]string{
		"name": "New Source",
		"url":  "https://news.example.test/feed.xml",
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create source status = %d, want %d", response.Code, http.StatusCreated)
	}
	source := decodeData[model.Source](t, response)
	if source.Format != "rss" || !source.Enabled || source.ArticleCount != 1 || source.LastFetchedAt == nil {
		t.Fatalf("new source = %+v, want fetched RSS source with one article", source)
	}

	response = performRequest(t, server.Handler(), http.MethodPost, "/api/v1/sources", map[string]string{
		"name": "Duplicate",
		"url":  "https://news.example.test/feed.xml",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate source status = %d, want %d", response.Code, http.StatusConflict)
	}

	response = performRequest(t, server.Handler(), http.MethodPost, "/api/v1/sources", map[string]string{
		"name": "Invalid",
		"url":  "file:///etc/passwd",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid URL status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestRefreshSource(t *testing.T) {
	memory := store.NewMemory()
	loader := staticFeedLoader{document: feed.Document{
		Format:  "atom",
		Entries: []feed.Entry{{ID: "entry-1", Title: "Refreshed article", Summary: "Summary"}},
	}}
	server := NewServerWithFeedLoader(memory, "", loader)

	response := performRequest(t, server.Handler(), http.MethodPost, "/api/v1/sources/orbit-journal/refresh", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Source        model.Source `json:"source"`
			AddedArticles int          `json:"addedArticles"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if envelope.Data.AddedArticles != 1 || envelope.Data.Source.Format != "atom" {
		t.Fatalf("refresh response = %+v", envelope.Data)
	}

	response = performRequest(t, server.Handler(), http.MethodPost, "/api/v1/sources/orbit-journal/refresh", nil)
	var second struct {
		Data struct {
			AddedArticles int `json:"addedArticles"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&second); err != nil {
		t.Fatalf("decode second refresh: %v", err)
	}
	if second.Data.AddedArticles != 0 {
		t.Fatalf("second refresh added = %d, want 0", second.Data.AddedArticles)
	}
}

func TestCORS(t *testing.T) {
	server := NewServer(store.NewMemory(), "http://localhost:5173")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func performRequest(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, &encoded)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeData[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return envelope.Data
}

type staticFeedLoader struct {
	document feed.Document
	err      error
}

func (loader staticFeedLoader) Load(context.Context, string) (feed.Document, error) {
	return loader.document, loader.err
}
