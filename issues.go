package main

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

// In-app issue / feedback reporting.
//
// Officers raise bugs, suggestions and questions from inside the app. Each
// report is stored locally (so there is a durable in-app record) together with
// a diagnostics snapshot — the build version/commit, the OS, and the most
// recent operational-log lines, which record actors and actions only and never
// citizen PII. Delivery is deliberately low-tech and credential-free: the app
// never talks to GitHub or an SMTP server itself. Instead it hands the officer
// a pre-composed email addressed to the contributors' group, which they send
// from their own mail client. The body carries a clearly delimited, machine-
// readable block so a future mailbox watcher (or the central-tier sync) can
// turn each report into a GitHub issue and round a status back into the app.

// defaultIssueEmail is the recipient seeded on first run; admins can change it
// in Settings. It is the contributors' Google Group.
const defaultIssueEmail = "egnnet-issues@googlegroups.com"

var issueTypes = map[string]bool{"bug": true, "suggestion": true, "question": true}
var issueStatuses = map[string]bool{"open": true, "sent": true, "acknowledged": true, "resolved": true}

// issueEmail reads the configured recipient, falling back to the default.
func (s *Server) issueEmail() string {
	var email string
	_ = s.db.QueryRow(`SELECT COALESCE(issue_email,'') FROM app_config WHERE id=1`).Scan(&email)
	if strings.TrimSpace(email) == "" {
		return defaultIssueEmail
	}
	return strings.TrimSpace(email)
}

// captureDiagnostics builds the delimited, parser-friendly block appended to
// each report's email body. Recent-activity lines come from app_log, which
// holds operational events only — never citizen data.
func (s *Server) captureDiagnostics(id, reportType, username, createdAt string) string {
	var b strings.Builder
	b.WriteString("----- eGNnet report (do not edit below) -----\n")
	fmt.Fprintf(&b, "report-id: %s\n", id)
	fmt.Fprintf(&b, "type: %s\n", reportType)
	fmt.Fprintf(&b, "app-version: %s\n", version)
	fmt.Fprintf(&b, "app-commit: %s\n", commit)
	fmt.Fprintf(&b, "app-env: %s\n", appEnv)
	fmt.Fprintf(&b, "os: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "reported-by: %s\n", username)
	fmt.Fprintf(&b, "reported-at: %s\n", createdAt)
	b.WriteString("recent-activity:\n")
	rows, _ := s.db.queryMaps(`SELECT created_at,level,category,action,detail FROM app_log
		ORDER BY created_at DESC LIMIT 30`)
	if len(rows) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "  %s [%s] %s · %s — %s\n",
			str(r["created_at"]), str(r["level"]), str(r["category"]), str(r["action"]), str(r["detail"]))
	}
	b.WriteString("----- end report -----")
	return b.String()
}

// emailFor assembles the {to, subject, body} the client turns into a mailto:.
func (s *Server) emailFor(reportType, title, description, diagnostics string) map[string]string {
	label := map[string]string{"bug": "Bug", "suggestion": "Suggestion", "question": "Question"}[reportType]
	if label == "" {
		label = "Report"
	}
	subject := fmt.Sprintf("[eGNnet] %s: %s", label, title)
	var body strings.Builder
	if strings.TrimSpace(description) != "" {
		body.WriteString(description)
		body.WriteString("\n\n")
	}
	body.WriteString(diagnostics)
	body.WriteString("\n")
	return map[string]string{"to": s.issueEmail(), "subject": subject, "body": body.String()}
}

// GET /api/issues — list reports, newest first.
func (s *Server) handleIssueList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.queryMaps(`SELECT id, report_type, title, description, status, synced,
		github_issue_number, app_version, username, created_at, updated_at
		FROM issue_report WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/issues {report_type, title, description}
func (s *Server) handleIssueCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		ReportType  string `json:"report_type"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title is required"})
		return
	}
	if !issueTypes[body.ReportType] {
		body.ReportType = "bug"
	}
	id := newID()
	now := nowISO()
	diag := s.captureDiagnostics(id, body.ReportType, sess.username, now)
	if _, err := s.db.Exec(`INSERT INTO issue_report
		(id,report_type,title,description,status,app_version,app_commit,app_env,os_info,diagnostics,user_id,username,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, body.ReportType, body.Title, body.Description, "open",
		version, commit, appEnv, runtime.GOOS+"/"+runtime.GOARCH, diag,
		sess.userID, sess.username, now, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "issue_report", id, "create", nil)
	if s.log != nil {
		s.log.write("info", "system", sess.userID, sess.username, "issue_reported",
			fmt.Sprintf("%s: %s", body.ReportType, body.Title))
	}
	email := s.emailFor(body.ReportType, body.Title, body.Description, diag)
	writeJSON(w, 200, map[string]any{"id": id, "email": email})
}

// GET /api/issues/{id}/email — rebuild the mailto payload for an existing report.
func (s *Server) handleIssueEmail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var reportType, title, description, diag string
	err := s.db.QueryRow(`SELECT report_type, title, COALESCE(description,''), COALESCE(diagnostics,'')
		FROM issue_report WHERE id=? AND deleted_at IS NULL`, id).Scan(&reportType, &title, &description, &diag)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, s.emailFor(reportType, title, description, diag))
}

// PUT /api/issues/{id} {status}
func (s *Server) handleIssueUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &body); err != nil || !issueStatuses[body.Status] {
		writeJSON(w, 400, map[string]string{"error": "invalid status"})
		return
	}
	res, err := s.db.Exec(`UPDATE issue_report SET status=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		body.Status, nowISO(), id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	s.db.audit(sess.userID, "issue_report", id, "update", map[string]string{"status": body.Status})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/issues/{id}
func (s *Server) handleIssueDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	s.db.Exec(`UPDATE issue_report SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "issue_report", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/issues/config — current recipient address.
func (s *Server) handleIssueConfigGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"issue_email": s.issueEmail()})
}

// PUT /api/issues/config {issue_email}
func (s *Server) handleIssueConfigSet(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		IssueEmail string `json:"issue_email"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	email := strings.TrimSpace(body.IssueEmail)
	if email == "" || !strings.Contains(email, "@") || strings.ContainsAny(email, " \t\r\n") {
		writeJSON(w, 400, map[string]string{"error": "enter a valid email address"})
		return
	}
	if _, err := s.db.Exec(`UPDATE app_config SET issue_email=?, updated_at=? WHERE id=1`, email, nowISO()); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if s.log != nil {
		s.log.write("info", "admin", sess.userID, sess.username, "issue_email_changed", email)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "issue_email": email})
}
