package main

import (
	"net/http"
	"strings"
)

var memberFields = []string{
	"serial_no", "full_name_en", "full_name_si", "relationship_to_head", "is_head", "title",
	"nic", "passport_no", "dob", "age", "gender", "civil_status", "ethnicity", "religion",
	"mother_language", "nationality", "residant_status", "birthplace", "education_level",
	"occupation", "occupation_detail", "job_status", "mobile", "email",
}

// POST /api/households/{id}/members
func (s *Server) handleMemberCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	hid := r.PathValue("id")
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	id := newID()
	now := nowISO()
	cols := []string{"id", "household_id", "created_at", "updated_at"}
	vals := []any{id, hid, now, now}
	for _, f := range memberFields {
		if v, ok := body[f]; ok {
			cols = append(cols, f)
			vals = append(vals, v)
		}
	}
	q := `INSERT INTO member(` + strings.Join(cols, ",") + `) VALUES(` + placeholders(len(cols)) + `)`
	if _, err := s.db.Exec(q, vals...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.applyMemberFlags(id, body, now)
	s.db.audit(sess.userID, "member", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

// PUT /api/members/{id}
func (s *Server) handleMemberUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	now := nowISO()
	set := []string{"updated_at=?"}
	args := []any{now}
	for _, f := range memberFields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE member SET `+strings.Join(set, ",")+` WHERE id=? AND deleted_at IS NULL`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, ok := body["flags"]; ok {
		s.db.Exec(`UPDATE member_flag SET deleted_at=? WHERE member_id=? AND deleted_at IS NULL`, now, id)
		s.applyMemberFlags(id, body, now)
	}
	s.db.audit(sess.userID, "member", id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/members/{id}
func (s *Server) handleMemberDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	s.db.Exec(`UPDATE member SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "member", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) applyMemberFlags(memberID string, body map[string]any, now string) {
	raw, ok := body["flags"].([]any)
	if !ok {
		return
	}
	for _, f := range raw {
		if ft, ok := f.(string); ok && ft != "" {
			s.db.Exec(`INSERT INTO member_flag(id,member_id,flag_type,created_at,updated_at) VALUES(?,?,?,?,?)`,
				newID(), memberID, ft, now, now)
		}
	}
}

// GET /api/households/{id}/welfare
func (s *Server) handleWelfareList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, _ := s.db.queryMaps(`SELECT we.*, p.name_en AS program_name FROM welfare_enrolment we
		JOIN welfare_program p ON p.id=we.program_id
		WHERE we.household_id=? AND we.deleted_at IS NULL`, id)
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/households/{id}/welfare
func (s *Server) handleWelfareCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	hid := r.PathValue("id")
	var body struct {
		ProgramID      string  `json:"program_id"`
		MemberID       string  `json:"member_id"`
		BeneficiaryRef string  `json:"beneficiary_ref"`
		Amount         float64 `json:"amount"`
		Status         string  `json:"status"`
	}
	if err := readJSON(r, &body); err != nil || body.ProgramID == "" {
		writeJSON(w, 400, map[string]string{"error": "program_id required"})
		return
	}
	id := newID()
	now := nowISO()
	var member any
	if body.MemberID != "" {
		member = body.MemberID
	}
	if body.Status == "" {
		body.Status = "active"
	}
	_, err := s.db.Exec(`INSERT INTO welfare_enrolment(id,household_id,member_id,program_id,beneficiary_ref,amount,status,start_date,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, id, hid, member, body.ProgramID, body.BeneficiaryRef, body.Amount, body.Status, now, now, now)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "welfare_enrolment", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

// DELETE /api/welfare/{id}
func (s *Server) handleWelfareDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	s.db.Exec(`UPDATE welfare_enrolment SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "welfare_enrolment", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/i18n/{lang}
func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if lang != "en" && lang != "si" && lang != "ta" {
		lang = "en"
	}
	b, err := webFS.ReadFile("internal/web/i18n/" + lang + ".json")
	if err != nil {
		b, _ = webFS.ReadFile("internal/web/i18n/en.json")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
}
