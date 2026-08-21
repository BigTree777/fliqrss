package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"fliqrss/backend/internal/feed"
	"fliqrss/backend/internal/model"
	"fliqrss/backend/internal/opml"
	"fliqrss/backend/internal/store"
)

const (
	maxRequestBody     = 1 << 20
	maxOPMLRequestBody = opml.DefaultMaxBytes + 256<<10
	opmlImportWorkers  = 8
)

var (
	errInvalidOPMLContentType = errors.New("OPML request must be multipart/form-data or XML")
	errMissingOPMLFile        = errors.New("OPML file is required")
	errOPMLTooLarge           = errors.New("OPML file is too large")
)

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
	store         store.Repository
	feedLoader    feed.Loader
	allowedOrigin string
	handler       http.Handler
}

func NewServer(repository store.Repository, allowedOrigin string) *Server {
	return NewServerWithFeedLoader(repository, allowedOrigin, feed.NewClient())
}

func NewServerWithFeedLoader(repository store.Repository, allowedOrigin string, loader feed.Loader) *Server {
	s := &Server{store: repository, feedLoader: loader, allowedOrigin: allowedOrigin}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/articles", s.listArticles)
	mux.HandleFunc("GET /api/v1/articles/page", s.listArticlePage)
	mux.HandleFunc("GET /api/v1/articles/stats", s.articleStats)
	mux.HandleFunc("POST /api/v1/articles/reset-skipped", s.resetSkipped)
	mux.HandleFunc("GET /api/v1/articles/{id}", s.getArticle)
	mux.HandleFunc("PATCH /api/v1/articles/{id}/state", s.updateArticleState)
	mux.HandleFunc("GET /api/v1/sources", s.listSources)
	mux.HandleFunc("POST /api/v1/sources", s.createSource)
	mux.HandleFunc("POST /api/v1/sources/import-opml", s.importOPML)
	mux.HandleFunc("GET /api/v1/sources/export-opml", s.exportOPML)
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
	filter, ok := articleFilterFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: s.store.ListArticles(filter)})
}

func (s *Server) listArticlePage(w http.ResponseWriter, r *http.Request) {
	filter, ok := articleFilterFromRequest(w, r)
	if !ok {
		return
	}
	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	page, err := s.store.ListArticlePage(filter, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: page})
}

func (s *Server) articleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dataResponse{Data: s.store.ArticleStats()})
}

func articleFilterFromRequest(w http.ResponseWriter, r *http.Request) (model.ArticleFilter, bool) {
	state := r.URL.Query().Get("state")
	if !validArticleStateFilter(state) {
		writeError(w, http.StatusBadRequest, "invalid_state", "state must be feed, all, favorite, saved, deleted, or read")
		return model.ArticleFilter{}, false
	}
	untagged := false
	if rawUntagged := r.URL.Query().Get("untagged"); rawUntagged != "" {
		parsed, err := strconv.ParseBool(rawUntagged)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_untagged", "untagged must be true or false")
			return model.ArticleFilter{}, false
		}
		untagged = parsed
	}
	return model.ArticleFilter{
		SourceID: r.URL.Query().Get("sourceId"),
		TagID:    r.URL.Query().Get("tagId"),
		Untagged: untagged,
		State:    state,
	}, true
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
	restored, err := s.store.ResetSkipped()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: map[string]int{"restored": restored}})
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
	source, _, err = s.store.UpsertArticles(source.ID, document.Format, feed.ArticlesFromDocument(source, document))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, dataResponse{Data: source})
}

type opmlImportResult struct {
	Total       int `json:"total"`
	Added       int `json:"added"`
	Duplicates  int `json:"duplicates"`
	Failed      int `json:"failed"`
	TagsCreated int `json:"tagsCreated"`
}

func (s *Server) importOPML(w http.ResponseWriter, r *http.Request) {
	payload, err := readOPMLPayload(w, r)
	if err != nil {
		switch {
		case errors.Is(err, errInvalidOPMLContentType):
			writeError(w, http.StatusUnsupportedMediaType, "invalid_content_type", err.Error())
		case errors.Is(err, errMissingOPMLFile):
			writeError(w, http.StatusBadRequest, "missing_opml_file", err.Error())
		case errors.Is(err, errOPMLTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "opml_too_large", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_opml_upload", "OPML upload could not be read")
		}
		return
	}

	subscriptions, err := opml.Parse(bytes.NewReader(payload))
	if err != nil {
		if errors.Is(err, opml.ErrLimitExceeded) {
			writeError(w, http.StatusRequestEntityTooLarge, "opml_limit_exceeded", "OPML contains too many elements or exceeds the maximum depth")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "invalid_opml", "file does not contain valid OPML")
		return
	}

	result := opmlImportResult{Total: len(subscriptions)}
	seenURLs := make(map[string]struct{}, len(subscriptions))
	jobs := make([]opml.Subscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		rawURL := strings.TrimSpace(subscription.XMLURL)
		urlKey := strings.ToLower(rawURL)
		if _, duplicate := seenURLs[urlKey]; duplicate || s.store.HasSourceURL(rawURL) {
			result.Duplicates++
			continue
		}
		seenURLs[urlKey] = struct{}{}
		if err := validateFeedURL(rawURL); err != nil {
			result.Failed++
			continue
		}
		subscription.XMLURL = rawURL
		jobs = append(jobs, subscription)
	}

	jobChannel := make(chan opml.Subscription)
	workerCount := min(opmlImportWorkers, len(jobs))
	var resultMutex sync.Mutex
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for subscription := range jobChannel {
				tagsCreated, duplicate, err := s.importOPMLSubscription(r.Context(), subscription)
				resultMutex.Lock()
				switch {
				case duplicate:
					result.Duplicates++
				case err != nil:
					result.Failed++
				default:
					result.Added++
					result.TagsCreated += tagsCreated
				}
				resultMutex.Unlock()
			}
		}()
	}
	for _, subscription := range jobs {
		jobChannel <- subscription
	}
	close(jobChannel)
	workers.Wait()

	writeJSON(w, http.StatusOK, dataResponse{Data: result})
}

func (s *Server) exportOPML(w http.ResponseWriter, _ *http.Request) {
	tagNames := make(map[string]string)
	for _, tag := range s.store.ListTags() {
		tagNames[tag.ID] = tag.Name
	}
	subscriptions := make([]opml.Subscription, 0)
	for _, source := range s.store.ListSources() {
		tags := make([]string, 0, len(source.TagIDs))
		for _, tagID := range source.TagIDs {
			if name := tagNames[tagID]; name != "" {
				tags = append(tags, name)
			}
		}
		subscriptions = append(subscriptions, opml.Subscription{
			Title:  source.Name,
			XMLURL: source.URL,
			Type:   source.Format,
			Tags:   tags,
		})
	}

	payload, err := opml.Marshal("fliqrss subscriptions", subscriptions, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "opml_export_failed", "OPML could not be generated")
		return
	}
	w.Header().Set("Content-Type", "text/x-opml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="fliqrss-subscriptions.opml"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		panic(fmt.Errorf("write OPML response: %w", err))
	}
}

func (s *Server) importOPMLSubscription(ctx context.Context, subscription opml.Subscription) (int, bool, error) {
	document, err := s.feedLoader.Load(ctx, subscription.XMLURL)
	if err != nil {
		return 0, false, err
	}
	name := strings.TrimSpace(subscription.Title)
	if name == "" {
		name = document.Title
	}
	if name == "" {
		return 0, false, errors.New("feed title is missing")
	}

	source, err := s.store.CreateSource(name, subscription.XMLURL, document.Format)
	if errors.Is(err, store.ErrConflict) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	source, _, err = s.store.UpsertArticles(source.ID, document.Format, feed.ArticlesFromDocument(source, document))
	if err != nil {
		return 0, false, err
	}

	tagsCreated := 0
	tagIDs := make([]string, 0, len(subscription.Tags))
	for _, tagName := range subscription.Tags {
		tag, created, err := s.store.FindOrCreateTag(tagName)
		if err != nil {
			return 0, false, err
		}
		if created {
			tagsCreated++
		}
		tagIDs = append(tagIDs, tag.ID)
	}
	if _, err := s.store.SetSourceTags(source.ID, tagIDs); err != nil {
		return 0, false, err
	}
	return tagsCreated, false, nil
}

func readOPMLPayload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxOPMLRequestBody)
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, errInvalidOPMLContentType
	}

	var reader io.Reader
	var closer io.Closer
	switch mediaType {
	case "multipart/form-data":
		multipartReader, err := r.MultipartReader()
		if err != nil {
			return nil, err
		}
		for {
			part, err := multipartReader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if part.FormName() == "file" {
				reader = part
				closer = part
				break
			}
			part.Close()
		}
		if reader == nil {
			return nil, errMissingOPMLFile
		}
	case "application/xml", "text/xml", "text/x-opml":
		reader = r.Body
	default:
		return nil, errInvalidOPMLContentType
	}
	if closer != nil {
		defer closer.Close()
	}

	payload, err := io.ReadAll(io.LimitReader(reader, opml.DefaultMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > opml.DefaultMaxBytes {
		return nil, errOPMLTooLarge
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, errMissingOPMLFile
	}
	return payload, nil
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
	source, added, err := s.store.UpsertArticles(source.ID, document.Format, feed.ArticlesFromDocument(source, document))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dataResponse{Data: map[string]any{
		"source":        source,
		"addedArticles": added,
	}})
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
	case errors.Is(err, store.ErrInvalidCursor):
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is no longer valid")
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
