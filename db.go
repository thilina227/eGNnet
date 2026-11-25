package main

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed internal/db/schema.sql
var schemaSQL string

//go:embed internal/db/seed.sql
var seedSQL string

//go:embed internal/db/divisions.sql
var divisionsSQL string

// DB wraps the SQLite connection.
type DB struct{ *sql.DB }

func openDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1) // SQLite: serialize writes, avoids "database is locked"
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return &DB{conn}, nil
}

// migrate applies the schema and seeds reference data on first run.
func (db *DB) migrate() error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	// add columns introduced after v0.1 (ignore "duplicate column" on re-run)
	_, _ = db.Exec(`ALTER TABLE app_config ADD COLUMN log_retention_days INTEGER NOT NULL DEFAULT 180`)
	_, _ = db.Exec(`ALTER TABLE app_config ADD COLUMN issue_email TEXT`)
	// ensure the single config row exists
	if _, err := db.Exec(`INSERT OR IGNORE INTO app_config(id,setup_complete,default_language,created_at,updated_at)
		VALUES(1,0,'en',datetime('now'),datetime('now'))`); err != nil {
		return fmt.Errorf("config row: %w", err)
	}
	// seed the default issue-report recipient once (admins can change it in Settings)
	_, _ = db.Exec(`UPDATE app_config SET issue_email=? WHERE id=1 AND (issue_email IS NULL OR issue_email='')`,
		defaultIssueEmail)
	// seed lookups/hierarchy only once
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lookup_option`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := db.Exec(seedSQL); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	// seed the full administrative hierarchy (island-wide MOHA dataset) once
	var g int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gn_division`).Scan(&g); err != nil {
		return err
	}
	if g == 0 {
		if _, err := db.Exec(divisionsSQL); err != nil {
			return fmt.Errorf("divisions: %w", err)
		}
	}
	return nil
}

// ---- helpers ----

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16]))
}

func nowISO() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// audit records a change for future sync.
func (db *DB) audit(userID, table, entityID, action string, changes any) {
	var payload string
	if changes != nil {
		if b, err := json.Marshal(changes); err == nil {
			payload = string(b)
		}
	}
	_, _ = db.Exec(`INSERT INTO audit_log(id,user_id,entity_table,entity_id,action,changes,created_at)
		VALUES(?,?,?,?,?,?,?)`, newID(), userID, table, entityID, action, payload, nowISO())
}
