package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const sessionKey ctxKey = "session"

func withSession(r *http.Request, s *session) context.Context {
	return context.WithValue(r.Context(), sessionKey, s)
}

func sessionFrom(r *http.Request) *session {
	if s, ok := r.Context().Value(sessionKey).(*session); ok {
		return s
	}
	return nil
}

func sessionToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GET /api/config — tells the UI whether setup is done and the default language.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	var complete int
	var lang, office string
	err := s.db.QueryRow(`SELECT setup_complete, default_language, COALESCE(office_name,'') FROM app_config WHERE id=1`).
		Scan(&complete, &lang, &office)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"setup_complete": complete == 1, "default_language": lang, "office_name": office,
	})
}

type setupReq struct {
	OfficerName string   `json:"officer_name"`
	NIC         string   `json:"nic"`
	Contact     string   `json:"contact"`
	Email       string   `json:"email"`
	DivisionIDs []string `json:"division_ids"`
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	Language    string   `json:"language"`
	OfficeName  string   `json:"office_name"`
}

// POST /api/setup — first-run: create the officer, their division assignments, and the admin user.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var complete int
	_ = s.db.QueryRow(`SELECT setup_complete FROM app_config WHERE id=1`).Scan(&complete)
	if complete == 1 {
		writeJSON(w, 409, map[string]string{"error": "already set up"})
		return
	}
	var req setupReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	if req.OfficerName == "" || req.Username == "" || len(req.Password) < 4 || len(req.DivisionIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "officer name, username, a password (4+ chars) and at least one division are required"})
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	now := nowISO()
	officerID := newID()
	tx, err := s.db.Begin()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO officer(id,full_name,nic,contact,email,started_date,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, officerID, req.OfficerName, req.NIC, req.Contact, req.Email, now, now, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for _, div := range req.DivisionIDs {
		if _, err := tx.Exec(`INSERT INTO officer_assignment(id,officer_id,gn_division_id,start_date,is_current,created_at,updated_at)
			VALUES(?,?,?,?,1,?,?)`, newID(), officerID, div, now, now, now); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid division: " + div})
			return
		}
	}
	userID := newID()
	if _, err := tx.Exec(`INSERT INTO app_user(id,officer_id,username,password_hash,role,scope_level,language_pref,is_active,created_at,updated_at)
		VALUES(?,?,?,?,'gn_officer','gn',?,1,?,?)`, userID, officerID, req.Username, string(hash), req.Language, now, now); err != nil {
		writeJSON(w, 409, map[string]string{"error": "username already exists"})
		return
	}
	if _, err := tx.Exec(`UPDATE app_config SET setup_complete=1, default_language=?, office_name=?, updated_at=? WHERE id=1`,
		req.Language, req.OfficeName, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(userID, "app_user", userID, "create", map[string]string{"setup": "complete"})
	if s.log != nil {
		s.log.write("info", "admin", userID, req.Username, "setup_complete", "office configured")
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	var id, hash, role string
	var active int
	err := s.db.QueryRow(`SELECT id,password_hash,role,is_active FROM app_user WHERE username=? AND deleted_at IS NULL`,
		req.Username).Scan(&id, &hash, &role, &active)
	if err != nil || active == 0 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		if s.log != nil {
			s.log.write("warn", "auth", "", req.Username, "login_failed", "invalid credentials")
		}
		writeJSON(w, 401, map[string]string{"error": "invalid username or password"})
		return
	}
	token := sessionToken()
	s.sessions.Store(token, &session{userID: id, username: req.Username, role: role, expires: time.Now().Add(12 * time.Hour)})
	s.db.Exec(`UPDATE app_user SET last_login=? WHERE id=?`, nowISO(), id)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 43200})
	if s.log != nil {
		s.log.write("info", "auth", id, req.Username, "login", "signed in")
	}
	writeJSON(w, 200, map[string]any{"ok": true, "role": role})
}

// POST /api/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := s.currentSession(r); sess != nil && s.log != nil {
		s.log.write("info", "auth", sess.userID, sess.username, "logout", "signed out")
	}
	if c, err := r.Cookie(cookieName); err == nil {
		s.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/me
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := s.currentSession(r)
	if sess == nil {
		writeJSON(w, 401, map[string]string{"error": "unauthenticated"})
		return
	}
	var lang, officer string
	_ = s.db.QueryRow(`SELECT u.language_pref, COALESCE(o.full_name,'') FROM app_user u
		LEFT JOIN officer o ON o.id=u.officer_id WHERE u.id=?`, sess.userID).Scan(&lang, &officer)
	writeJSON(w, 200, map[string]any{"username": sess.username, "role": sess.role, "language": lang, "officer_name": officer})
}
