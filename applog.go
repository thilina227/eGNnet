package main

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// appLogger writes operational events both to the app_log table (for the
// in-app viewer) and to a rolling JSONL file in the app-data folder (for
// durability and export). Old file segments are compressed and, together
// with old app_log/audit_log rows, pruned to a configurable retention window.
//
// It records actors and actions only — never citizen PII.
type appLogger struct {
	db      *DB
	dir     string // <app-data>/logs
	file    string // <dir>/app.log (active segment)
	maxSize int64  // rotate when the active segment exceeds this
	mu      sync.Mutex
	stop    chan struct{}
}

func newAppLogger(db *DB, appDataDir string) *appLogger {
	dir := filepath.Join(appDataDir, "logs")
	_ = os.MkdirAll(dir, 0o755)
	return &appLogger{
		db:      db,
		dir:     dir,
		file:    filepath.Join(dir, "app.log"),
		maxSize: 5 << 20, // 5 MiB
		stop:    make(chan struct{}),
	}
}

// write records one operational event. All I/O is best-effort: logging must
// never break a request.
func (l *appLogger) write(level, category, userID, username, action, detail string) {
	if level == "" {
		level = "info"
	}
	if category == "" {
		category = "system"
	}
	ts := nowISO()
	// 1) in-app table
	if l.db != nil {
		_, _ = l.db.Exec(`INSERT INTO app_log(id,level,category,user_id,username,action,detail,created_at)
			VALUES(?,?,?,?,?,?,?,?)`, newID(), level, category, nullIfEmpty(userID), username, action, detail, ts)
	}
	// 2) rolling file (JSONL)
	l.mu.Lock()
	defer l.mu.Unlock()
	line, _ := json.Marshal(map[string]string{
		"ts": ts, "level": level, "category": category,
		"username": username, "action": action, "detail": detail,
	})
	f, err := os.OpenFile(l.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
	_ = f.Close()
	l.rotateIfNeeded()
}

// rotateIfNeeded compresses the active segment into logs/app-<ts>.log.gz once
// it grows past maxSize. Caller holds l.mu.
func (l *appLogger) rotateIfNeeded() {
	fi, err := os.Stat(l.file)
	if err != nil || fi.Size() < l.maxSize {
		return
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	dst := filepath.Join(l.dir, "app-"+stamp+".log.gz")
	if err := gzipFile(l.file, dst); err != nil {
		return
	}
	_ = os.Truncate(l.file, 0)
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

// retentionDays reads the configured window (falls back to 180).
func (l *appLogger) retentionDays() int {
	days := 180
	if l.db != nil {
		_ = l.db.QueryRow(`SELECT log_retention_days FROM app_config WHERE id=1`).Scan(&days)
	}
	if days < 1 {
		days = 1
	}
	return days
}

// sweep prunes app_log rows, audit_log rows, and archived .gz segments older
// than the retention window. Returns how many of each were removed.
func (l *appLogger) sweep() (appRows, auditRows, files int64) {
	days := l.retentionDays()
	cutoffISO := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02T15:04:05Z")

	if l.db != nil {
		if res, err := l.db.Exec(`DELETE FROM app_log WHERE created_at < ?`, cutoffISO); err == nil {
			appRows, _ = res.RowsAffected()
		}
		if res, err := l.db.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoffISO); err == nil {
			auditRows, _ = res.RowsAffected()
		}
	}

	cutoffT := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	entries, _ := os.ReadDir(l.dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log.gz") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().Before(cutoffT) {
			if os.Remove(filepath.Join(l.dir, e.Name())) == nil {
				files++
			}
		}
	}
	return
}

// start runs an initial sweep and then a daily one until stop is closed.
func (l *appLogger) start() {
	go func() {
		l.sweep()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.sweep()
			case <-l.stop:
				return
			}
		}
	}()
}

// archives lists the compressed segments, newest first.
func (l *appLogger) archives() []map[string]any {
	entries, _ := os.ReadDir(l.dir)
	var out []map[string]any
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log.gz") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, map[string]any{
			"name":        e.Name(),
			"size_bytes":  fi.Size(),
			"modified_at": fi.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["name"].(string) > out[j]["name"].(string)
	})
	return out
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
