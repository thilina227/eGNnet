package main

import (
	"net/http"
	"strings"
)

// Work diary / day-book: the officer's dated log of office work, division-scoped.

// diaryDivisionVisible reports whether the entry exists in a division the user may see.
func (s *Server) diaryDivisionVisible(userID, entryID string) bool {
	var div string
	if err := s.db.QueryRow(`SELECT gn_division_id FROM diary_entry WHERE id=? AND deleted_at IS NULL`, entryID).Scan(&div); err != nil {
		return false
	}
	return contains(s.divisionIDsForUser(userID), div)
}

// GET /api/diary?division=&from=&to=&q=
func (s *Server) handleDiaryList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	divs := s.divisionIDsForUser(sess.userID)
	if len(divs) == 0 {
		writeJSON(w, 200, []any{})
		return
	}
	where := []string{"d.deleted_at IS NULL"}
	args := []any{}
	if division := r.URL.Query().Get("division"); division != "" && contains(divs, division) {
		where = append(where, "d.gn_division_id=?")
		args = append(args, division)
	} else {
		where = append(where, "d.gn_division_id IN ("+placeholders(len(divs))+")")
		args = append(args, toAny(divs)...)
	}
	if v := r.URL.Query().Get("from"); v != "" {
		where = append(where, "d.entry_date >= ?")
		args = append(args, v)
	}
	if v := r.URL.Query().Get("to"); v != "" {
		where = append(where, "d.entry_date <= ?")
		args = append(args, v)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		where = append(where, "(d.title LIKE ? OR d.note LIKE ? OR d.category LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.db.queryMaps(`SELECT d.id, d.gn_division_id, d.entry_date, d.category, d.title, d.note,
		COALESCE(o.full_name, u.username, '') AS author, g.gn_code, g.name_en AS division_name
		FROM diary_entry d
		LEFT JOIN app_user u ON u.id=d.user_id
		LEFT JOIN officer o ON o.id=u.officer_id
		JOIN gn_division g ON g.id=d.gn_division_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY d.entry_date DESC, d.created_at DESC LIMIT 1000`, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/diary  {gn_division_id, entry_date, category, title, note}
func (s *Server) handleDiaryCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		DivisionID string `json:"gn_division_id"`
		EntryDate  string `json:"entry_date"`
		Category   string `json:"category"`
		Title      string `json:"title"`
		Note       string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil || body.DivisionID == "" {
		writeJSON(w, 400, map[string]string{"error": "gn_division_id is required"})
		return
	}
	if !contains(s.divisionIDsForUser(sess.userID), body.DivisionID) {
		writeJSON(w, 403, map[string]string{"error": "division not assigned to you"})
		return
	}
	now := nowISO()
	if body.EntryDate == "" {
		body.EntryDate = now[:10]
	}
	if strings.TrimSpace(body.Title) == "" && strings.TrimSpace(body.Note) == "" {
		writeJSON(w, 400, map[string]string{"error": "enter a title or a note"})
		return
	}
	id := newID()
	if _, err := s.db.Exec(`INSERT INTO diary_entry(id,gn_division_id,user_id,entry_date,category,title,note,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, id, body.DivisionID, sess.userID, body.EntryDate, body.Category, body.Title, body.Note, now, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "diary_entry", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

var diaryFields = []string{"entry_date", "category", "title", "note"}

// PUT /api/diary/{id}
func (s *Server) handleDiaryUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if !s.diaryDivisionVisible(sess.userID, id) {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	set := []string{"updated_at=?"}
	args := []any{nowISO()}
	for _, f := range diaryFields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE diary_entry SET `+strings.Join(set, ",")+` WHERE id=?`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "diary_entry", id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/diary/{id}
func (s *Server) handleDiaryDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if !s.diaryDivisionVisible(sess.userID, id) {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	s.db.Exec(`UPDATE diary_entry SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "diary_entry", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
