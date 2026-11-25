package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// GET /api/admin/logs?level=&category=&q=&limit=
// Returns recent operational-log rows for the in-app viewer (newest first).
func (s *Server) handleLogList(w http.ResponseWriter, r *http.Request) {
	where := []string{"1=1"}
	args := []any{}
	if v := r.URL.Query().Get("level"); v != "" {
		where = append(where, "level=?")
		args = append(args, v)
	}
	if v := r.URL.Query().Get("category"); v != "" {
		where = append(where, "category=?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		where = append(where, "(action LIKE ? OR detail LIKE ? OR username LIKE ?)")
		like := "%" + v + "%"
		args = append(args, like, like, like)
	}
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	rows, err := s.db.queryMaps(`SELECT id,created_at,level,category,username,action,detail FROM app_log
		WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, orEmpty(rows))
}

// GET /api/admin/log-config — retention window, current volumes, archives.
func (s *Server) handleLogConfigGet(w http.ResponseWriter, r *http.Request) {
	var days, appCount, auditCount int
	_ = s.db.QueryRow(`SELECT log_retention_days FROM app_config WHERE id=1`).Scan(&days)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM app_log`).Scan(&appCount)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&auditCount)
	archives := []map[string]any{}
	if s.log != nil {
		archives = s.log.archives()
	}
	writeJSON(w, 200, map[string]any{
		"retention_days":  days,
		"app_log_count":   appCount,
		"audit_log_count": auditCount,
		"archives":        archives,
	})
}

// PUT /api/admin/log-config {retention_days}
func (s *Server) handleLogConfigSet(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	if body.RetentionDays < 1 || body.RetentionDays > 3650 {
		writeJSON(w, 400, map[string]string{"error": "retention_days must be between 1 and 3650"})
		return
	}
	if _, err := s.db.Exec(`UPDATE app_config SET log_retention_days=?, updated_at=? WHERE id=1`,
		body.RetentionDays, nowISO()); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if s.log != nil {
		s.log.write("info", "admin", sess.userID, sess.username, "retention_changed",
			fmt.Sprintf("log retention set to %d days", body.RetentionDays))
		// apply the new window immediately
		s.log.sweep()
	}
	writeJSON(w, 200, map[string]any{"ok": true, "retention_days": body.RetentionDays})
}

// POST /api/admin/logs/sweep — prune now to the retention window.
func (s *Server) handleLogSweep(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if s.log == nil {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	appRows, auditRows, files := s.log.sweep()
	s.log.write("info", "admin", sess.userID, sess.username, "retention_sweep",
		fmt.Sprintf("pruned %d app-log rows, %d audit rows, %d archives", appRows, auditRows, files))
	writeJSON(w, 200, map[string]any{
		"ok": true, "app_log_removed": appRows, "audit_log_removed": auditRows, "archives_removed": files,
	})
}

// GET /api/admin/logs/export — download the operational log as CSV.
// Actors + actions only; no citizen PII.
func (s *Server) handleLogExport(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	rows, err := s.db.queryMaps(`SELECT created_at,level,category,username,action,detail FROM app_log
		ORDER BY created_at DESC LIMIT 100000`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="egnnet-app-log.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"timestamp", "level", "category", "username", "action", "detail"})
	for _, row := range rows {
		_ = cw.Write([]string{
			str(row["created_at"]), str(row["level"]), str(row["category"]),
			str(row["username"]), str(row["action"]), str(row["detail"]),
		})
	}
	cw.Flush()
	if s.log != nil {
		s.log.write("info", "admin", sess.userID, sess.username, "log_exported",
			fmt.Sprintf("exported %d rows to CSV", len(rows)))
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
