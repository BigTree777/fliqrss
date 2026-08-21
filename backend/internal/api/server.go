package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/model"
	"fliqrss/backend/internal/store"
)

const maxRequestBody = 1 << 20

type dataResponse struct {
	Data any `json:"data"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type Server struct {
	store         *store.Memory
	feedLoader    feed.Loader
	allowedOrigin string
	handler       http.Handler
}

func NewServer(memory *store.Memory, allowedOrigin string) *Server {
	return NewServerWithFeedLoader(memory, allowedOrigin, feed.NewClient())
}

func NewServerWithFeedLoader(memory *store.Memory, allowedOrigin string, loader feed.Loader) *Server {
	s := &Server{store: memory, feedLoader: loader, allowedOrigin: allowedOrigin}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/articles", s.listArticles)
	mux.HandleFunc("POST /api/v1/articles/reset-skipped", s.resetSkipped)
	mux.HandleFunc("GET /api/v1/articles/{id}", s.getArticle)
	mux.HandleFunc("PATCH /api/v1/articles/{id}/state", s.updateArticleState)
	mux.HandleFunc("GET /api/v1/sources", s.listSources)
	mux.HandleFunc("POST /api/v1/sources", s.createSource)
	mux.HandleFunc("PATCH /api/v1/sources/{id}", s.updateSource)
	mux.HandleFunc("DELETE /api/v1/sources/{id}", s.deleteSource)
	mux.HandleFunc("POST /api/v1/sources/{id}/refresh", s.refreshSource)
	mux.HandleFunc("PUT /api/v1/sources/{id}/tags", s.setSourceTags)
	mux.HandleFunc("GET /api/v1/tags", s.listTags)
	mux.HandleFunc("POST /api/v1/tags", s.createTag)
	mux.HandleFunc("PATCH /api/v1/tags/{id}", s.updateTag)
	mux.HandleFunc("DELETE /api/v1/tags/{id}", s.deleteTag)
	mux.HandleFunc("OPTIONS /api/", s.preflight)
	s.handler = s.withCORS(mux)
	return s
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dataResponse{Data: map[string]string{"status": "ok"}})
}

func (s *Server) listArticles(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if !validArticleStateFilter(state) {
		writeError(w, http.StatusBadRequest, "invalid_state", "state must be feed, all, favorite, saved, deleted, or read")
		return
	}
	filter := model.ArticleFilter{
		SourceID: r.URL.Query().Get("sourceId"),
		TagID:    r.URL.Query().Get("tagId"),
		State:    state,
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: s.store.ListArticles(filter)})
}

func validArticleStateFilter(state string) bool {
	switch state {
	case "", "feed", "all", "favorite", "saved", "deleted", "read":
		return true
	default:
		return false
	}
}

func (s *Server) getArticle(w http.ResponseWriter, r *http.Request) {
	article, err := s.store.GetArticle(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: article})
}

func (s *Server) updateArticleState(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Action model.ArticleAction `json:"action"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	article, err := s.store.ApplyArticleAction(r.PathValue("id"), request.Action)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: article})
}

func (s *Server) resetSkipped(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dataResponse{Data: map[string]int{"restored": s.store.ResetSkipped()}})
}

func (s *Server) listSources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dataResponse{Data: s.store.ListSources()})
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	request.URL = strings.TrimSpace(request.URL)
	if err := validateFeedURL(request.URL); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}
	if s.store.HasSourceURL(request.URL) {
		writeError(w, http.StatusConflict, "conflict", "source URL is already registered")
		return
	}
	document, err := s.feedLoader.Load(r.Context(), request.URL)
	if err != nil {
		writeFeedError(w, err)
		return
	}
	if request.Name == "" {
		request.Name = document.Title
	}
	if request.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "missing_feed_title", "feed has no title, so name must be provided")
		return
	}
	source, err := s.store.CreateSource(request.Name, request.URL, document.Format)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	source, _, err = s.store.UpsertArticles(source.ID, document.Format, articlesFromFeed(source, document))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dataResponse{Data: source})
}

func validateFeedURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return errors.New("url must be an absolute HTTP or HTTPS URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use HTTP or HTTPS")
	}
	if parsed.User != nil {
		return errors.New("url must not include user information")
	}
	return nil
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	if request.Name == nil && request.Enabled == nil {
		writeError(w, http.StatusBadRequest, "empty_update", "name or enabled is required")
		return
	}
	if request.Name != nil {
		trimmed := strings.TrimSpace(*request.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "invalid_name", "name must not be empty")
			return
		}
		request.Name = &trimmed
	}
	source, err := s.store.UpdateSource(r.PathValue("id"), request.Name, request.Enabled)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: source})
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSource(r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshSource(w http.ResponseWriter, r *http.Request) {
	source, err := s.store.GetSource(r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !source.Enabled {
		writeError(w, http.StatusConflict, "source_disabled", "source is disabled")
		return
	}
	document, err := s.feedLoader.Load(r.Context(), source.URL)
	if err != nil {
		writeFeedError(w, err)
		return
	}
	source, added, err := s.store.UpsertArticles(source.ID, document.Format, articlesFromFeed(source, document))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: map[string]any{
		"source":        source,
		"addedArticles": added,
	}})
}

func articlesFromFeed(source model.Source, document feed.Document) []model.Article {
	articles := make([]model.Article, 0, len(document.Entries))
	for _, entry := range document.Entries {
		summary := entry.Summary
		if summary == "" {
			summary = entry.Content
		}
		body := []string{}
		if entry.Content != "" && entry.Content != summary {
			body = append(body, entry.Content)
		}
		publishedAt := entry.PublishedAt
		if publishedAt == "" {
			publishedAt = time.Now().UTC().Format(time.RFC3339)
		}
		articles = append(articles, model.Article{
			ID:             source.ID + "-" + strings.TrimPrefix(entry.ID, "feed-"),
			SourceID:       source.ID,
			Source:         source.Name,
			SourceInitials: sourceInitials(source.Name),
			PublishedAt:    publishedAt,
			ReadTime:       feed.EstimateReadTime(summary, entry.Content),
			Title:          entry.Title,
			URL:            entry.Link,
			Summary:        summary,
			Body:           body,
			VisualLabel:    strings.ToUpper(document.Format) + " FEED",
			VisualTheme:    visualTheme(entry.ID),
		})
	}
	return articles
}

func sourceInitials(name string) string {
	words := strings.Fields(name)
	var initials []rune
	for _, word := range words {
		character, _ := utf8.DecodeRuneInString(word)
		if character != utf8.RuneError {
			initials = append(initials, character)
		}
		if len(initials) == 2 {
			break
		}
	}
	if len(initials) == 1 {
		runes := []rune(name)
		if len(runes) > 1 {
			initials = append(initials, runes[1])
		}
	}
	return strings.ToUpper(string(initials))
}

func visualTheme(identity string) string {
	themes := []string{"cobalt", "coral", "forest", "violet", "amber", "aqua"}
	value := 0
	for _, character := range identity {
		value = (value*31 + int(character)) % len(themes)
	}
	return themes[value]
}

func (s *Server) setSourceTags(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TagIDs []string `json:"tagIds"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	if request.TagIDs == nil {
		request.TagIDs = []string{}
	}
	source, err := s.store.SetSourceTags(r.PathValue("id"), request.TagIDs)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: source})
}

func (s *Server) listTags(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dataResponse{Data: s.store.ListTags()})
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_name", "name is required")
		return
	}
	tag, err := s.store.CreateTag(request.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dataResponse{Data: tag})
}

func (s *Server) updateTag(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeRequest(w, r, &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_name", "name is required")
		return
	}
	tag, err := s.store.UpdateTag(r.PathValue("id"), request.Name)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: tag})
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTag(r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) preflight(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.allowedOrigin != "" && r.Header.Get("Origin") == s.allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", s.allowedOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	if mediaType := r.Header.Get("Content-Type"); mediaType != "" && !strings.HasPrefix(strings.ToLower(mediaType), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "invalid_content_type", "Content-Type must be application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain valid JSON")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "requested resource was not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists")
	case errors.Is(err, store.ErrBadAction):
		writeError(w, http.StatusBadRequest, "invalid_action", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func writeFeedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, feed.ErrUnsafeURL):
		writeError(w, http.StatusBadRequest, "unsafe_feed_url", "feed URL points to a blocked network address")
	case errors.Is(err, feed.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "feed_too_large", "feed exceeds the maximum response size")
	case errors.Is(err, feed.ErrUnsupportedFormat):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_feed", "URL does not contain a supported RSS or Atom feed")
	default:
		writeError(w, http.StatusBadGateway, "feed_fetch_failed", "feed could not be fetched or parsed")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(fmt.Errorf("encode JSON response: %w", err))
	}
}
