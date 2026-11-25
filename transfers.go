package main

import (
	"net/http"
	"strings"
)

// Transfer-of-work log: handover of pending work/files/duties between officers,
// division-scoped. officer_assignment already holds the posting history; this
// records the work handed over.

func (s *Server) transferVisible(userID, id string) bool {
	var div string
	if err := s.db.QueryRow(`SELECT gn_division_id FROM work_transfer WHERE id=? AND deleted_at IS NULL`, id).Scan(&div); err != nil {
		return false
	}
	return contains(s.divisionIDsForUser(userID), div)
}

// GET /api/transfers?division=&status=&q=
func (s *Server) handleTransferList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	divs := s.divisionIDsForUser(sess.userID)
	if len(divs) == 0 {
		writeJSON(w, 200, []any{})
		return
	}
	where := []string{"tr.deleted_at IS NULL"}
	args := []any{}
	if division := r.URL.Query().Get("division"); division != "" && contains(divs, division) {
		where = append(where, "tr.gn_division_id=?")
		args = append(args, division)
	} else {
		where = append(where, "tr.gn_division_id IN ("+placeholders(len(divs))+")")
		args = append(args, toAny(divs)...)
	}
	if v := r.URL.Query().Get("status"); v == "pending" || v == "completed" {
		where = append(where, "tr.status=?")
		args = append(args, v)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		where = append(where, "(tr.subject LIKE ? OR tr.note LIKE ? OR tr.from_officer LIKE ? OR tr.to_officer LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	rows, err := s.db.queryMaps(`SELECT tr.id, tr.gn_division_id, tr.transfer_date, tr.from_officer, tr.to_officer,
		tr.subject, tr.note, tr.status, g.gn_code, g.name_en AS division_name
		FROM work_transfer tr JOIN gn_division g ON g.id=tr.gn_division_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY tr.transfer_date DESC, tr.created_at DESC LIMIT 1000`, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/transfers
func (s *Server) handleTransferCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		DivisionID   string `json:"gn_division_id"`
		TransferDate string `json:"transfer_date"`
		FromOfficer  string `json:"from_officer"`
		ToOfficer    string `json:"to_officer"`
		Subject      string `json:"subject"`
		Note         string `json:"note"`
		Status       string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil || body.DivisionID == "" {
		writeJSON(w, 400, map[string]string{"error": "gn_division_id is required"})
		return
	}
	if !contains(s.divisionIDsForUser(sess.userID), body.DivisionID) {
		writeJSON(w, 403, map[string]string{"error": "division not assigned to you"})
		return
	}
	if strings.TrimSpace(body.Subject) == "" {
		writeJSON(w, 400, map[string]string{"error": "subject is required"})
		return
	}
	now := nowISO()
	if body.TransferDate == "" {
		body.TransferDate = now[:10]
	}
	if body.Status != "completed" {
		body.Status = "pending"
	}
	id := newID()
	if _, err := s.db.Exec(`INSERT INTO work_transfer(id,gn_division_id,transfer_date,from_officer,to_officer,subject,note,status,user_id,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, body.DivisionID, body.TransferDate, body.FromOfficer, body.ToOfficer, body.Subject, body.Note, body.Status, sess.userID, now, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "work_transfer", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

var transferFields = []string{"transfer_date", "from_officer", "to_officer", "subject", "note", "status"}

// PUT /api/transfers/{id}
func (s *Server) handleTransferUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if !s.transferVisible(sess.userID, id) {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	if st, ok := body["status"].(string); ok && st != "pending" && st != "completed" {
		writeJSON(w, 400, map[string]string{"error": "status must be pending or completed"})
		return
	}
	set := []string{"updated_at=?"}
	args := []any{nowISO()}
	for _, f := range transferFields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE work_transfer SET `+strings.Join(set, ",")+` WHERE id=?`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "work_transfer", id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/transfers/{id}
func (s *Server) handleTransferDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if !s.transferVisible(sess.userID, id) {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	s.db.Exec(`UPDATE work_transfer SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "work_transfer", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
