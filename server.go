package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed internal/web
var webFS embed.FS

// Server holds dependencies and in-memory sessions.
type Server struct {
	db       *DB
	sessions sync.Map // token -> session
}

type session struct {
	userID   string
	username string
	role     string
	expires  time.Time
}

func newServer(db *DB) *Server { return &Server{db: db} }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// --- public endpoints ---
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("GET /api/i18n/{lang}", s.handleI18n)
	mux.HandleFunc("GET /api/provinces", s.handleProvinces)
	mux.HandleFunc("GET /api/districts", s.handleDistricts)
	mux.HandleFunc("GET /api/ds-divisions", s.handleDSDivisions)
	mux.HandleFunc("GET /api/gn-divisions", s.handleGNDivisions)

	// --- authenticated endpoints ---
	mux.Handle("GET /api/lookups", s.auth(s.handleLookups))
	mux.Handle("GET /api/stats", s.auth(s.handleStats))
	mux.Handle("GET /api/households", s.auth(s.handleHouseholdList))
	mux.Handle("POST /api/households", s.auth(s.handleHouseholdCreate))
	mux.Handle("GET /api/households/{id}", s.auth(s.handleHouseholdGet))
	mux.Handle("PUT /api/households/{id}", s.auth(s.handleHouseholdUpdate))
	mux.Handle("DELETE /api/households/{id}", s.auth(s.handleHouseholdDelete))
	mux.Handle("POST /api/households/{id}/members", s.auth(s.handleMemberCreate))
	mux.Handle("PUT /api/members/{id}", s.auth(s.handleMemberUpdate))
	mux.Handle("DELETE /api/members/{id}", s.auth(s.handleMemberDelete))
	mux.Handle("GET /api/households/{id}/welfare", s.auth(s.handleWelfareList))
	mux.Handle("POST /api/households/{id}/welfare", s.auth(s.handleWelfareCreate))
	mux.Handle("DELETE /api/welfare/{id}", s.auth(s.handleWelfareDelete))
	mux.Handle("GET /api/welfare-programs", s.auth(s.handleWelfarePrograms))
	mux.Handle("GET /api/my-divisions", s.auth(s.handleMyDivisions))

	// --- static UI ---
	sub, _ := fs.Sub(webFS, "internal/web")
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// serve SPA: real files pass through, everything else returns index.html
		if r.URL.Path != "/" && !strings.Contains(r.URL.Path, "..") {
			if _, err := fs.Stat(sub, strings.TrimPrefix(r.URL.Path, "/")); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		b, _ := fs.ReadFile(sub, "index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})

	return logRequests(mux)
}

// ---- middleware & helpers ----

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { next.ServeHTTP(w, r) })
}

const cookieName = "egn_session"

func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := s.currentSession(r)
		if sess == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		r = r.WithContext(withSession(r, sess))
		h(w, r)
	})
}

func (s *Server) currentSession(r *http.Request) *session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	v, ok := s.sessions.Load(c.Value)
	if !ok {
		return nil
	}
	sess := v.(*session)
	if time.Now().After(sess.expires) {
		s.sessions.Delete(c.Value)
		return nil
	}
	return sess
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
