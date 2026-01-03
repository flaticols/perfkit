package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/flaticols/perfkit/internal/config"
	"github.com/flaticols/perfkit/internal/storage"
	"github.com/flaticols/perfkit/internal/ui"
)

type Server struct {
	cfg     *config.Config
	store   *storage.Store
	httpSrv *http.Server
}

func New(cfg *config.Config, store *storage.Store) *Server {
	return &Server{
		cfg:   cfg,
		store: store,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/pprof/ingest", s.handlePprofIngest)
	mux.HandleFunc("GET /api/profiles", s.handleListProfiles)
	mux.HandleFunc("GET /api/profiles/compare", s.handleCompareProfiles)
	mux.HandleFunc("GET /api/profiles/{id}", s.handleGetProfile)

	// Static files and UI
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(ui.StaticFS()))))
	mux.Handle("GET /fonts/", http.StripPrefix("/fonts/", http.FileServer(http.FS(ui.FontsFS()))))
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /profile/{id}", s.handleIndex)
	mux.HandleFunc("GET /compare/{ids}", s.handleIndex)

	// pprof endpoints for self-profiling
	if s.cfg.Server.EnablePprof {
		log.Println("pprof endpoints enabled at /debug/pprof/")
		mux.HandleFunc("GET /debug/pprof/", pprof.Index)
		mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
		mux.Handle("GET /debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("GET /debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("GET /debug/pprof/block", pprof.Handler("block"))
		mux.Handle("GET /debug/pprof/mutex", pprof.Handler("mutex"))
		mux.Handle("GET /debug/pprof/allocs", pprof.Handler("allocs"))
		mux.Handle("GET /debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Starting server on %s", addr)
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	f, err := ui.StaticFS().Open("index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.Copy(w, f)
}
