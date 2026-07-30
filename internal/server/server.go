package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/search"
	"github.com/jason/incipit/internal/storage"
)

type Server struct {
	DB       *db.DB
	Storage  *storage.Storage
	Config   config.Config
	Handler  http.Handler
	searcher search.Searcher
}

func New(cfg config.Config) (*Server, error) {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(); err != nil {
		database.Close()
		return nil, err
	}

	store := storage.New(cfg.StorageDir)

	s := &Server{
		DB:       database,
		Storage:  store,
		Config:   cfg,
		searcher: search.NewLikeSearcher(database),
	}

	s.Handler = s.router()
	return s, nil
}

func (s *Server) router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)
	r.Use(securityHeaders)
	r.Use(maxBodySize(10 << 20)) // 10MB default body limit

	r.Get("/health", s.health)
	r.Get("/syncs/healthcheck", s.syncHealthcheck)
	r.Get("/opds/opensearch.xml", s.opensearchDescription)
	r.Handle("/static/*", staticFileServer())

	r.Group(func(r chi.Router) {
		r.Use(rateLimit)
		r.Use(s.basicAuth)
		r.Use(s.csrfProtect)

		// API routes — JSON, no CSRF needed (basic auth protects them)
		r.Get("/syncs/auth", s.syncAuth)
		r.Get("/syncs/progress/{hash}", s.getProgress)
		r.Put("/syncs/progress/{hash}", s.putProgress)
		r.Post("/api/tags", s.apiCreateTag)
		r.Put("/api/tags/{id}", s.apiUpdateTag)
		r.Delete("/api/tags/{id}", s.apiDeleteTag)
		r.Post("/api/series/rename", s.apiRenameSeries)
		r.Post("/api/books/{id}/cover", s.uploadCover)
		r.Get("/api/books", s.handleListBooks)
		r.Get("/api/books/{id}", s.handleGetBook)
		r.Put("/api/books/{id}", s.handleUpdateBook)
		r.Delete("/api/books/{id}", s.handleDeleteBook)
		r.Get("/api/tags", s.handleListTags)
		r.Get("/api/series", s.handleListSeries)
		r.Get("/api/lookup", s.handleLookup)

		// OPDS routes — read-only catalog
		r.Get("/opds", s.opdsRoot)
		r.Get("/opds/newest", s.opdsNewest)
		r.Get("/opds/byauthor", s.opdsByAuthor)
		r.Get("/opds/byauthor/{author}", s.opdsByAuthorBooks)
		r.Get("/opds/byseries", s.opdsBySeries)
		r.Get("/opds/byseries/{series}", s.opdsBySeriesBooks)
		r.Get("/opds/bytag", s.opdsByTag)
		r.Get("/opds/bytag/{tag}", s.opdsByTagBooks)
		r.Get("/opds/search", s.opdsSearch)
		r.Get("/opds/book/{id}/download", s.opdsDownload)

		// File/cover serving — read-only
		r.Get("/covers/{id}", s.serveCover)
		r.Get("/files/{id}", s.serveFile)

		// Web form routes — CSRF protected, larger body limit for uploads
		r.With(maxBodySize(50<<20)).Post("/upload", s.handleUpload)
		r.With(maxBodySize(50<<20)).Post("/book/{id}/cover", s.uploadCoverRedirect)
		r.Post("/book/{id}/edit", s.editBookSave)
		r.Post("/book/{id}/delete", s.deleteBookPage)

		// Web UI pages — read-only
		r.Get("/", s.indexPage)
		r.Get("/book/{id}", s.bookPage)
		r.Get("/book/{id}/edit", s.editBookPage)
		r.Get("/tags", s.tagsPage)
		r.Get("/series", s.seriesPage)
		r.Get("/upload", s.uploadPage)
	})

	return r
}

func (s *Server) Run() error {
	srv := &http.Server{
		Addr:    ":" + s.Config.Port,
		Handler: s.Handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("incipit serving on :%s", s.Config.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down...")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}

	s.DB.Close()
	log.Println("bye")
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %dms", r.Method, r.URL.Path, ww.Status(), time.Since(start).Milliseconds())
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}
