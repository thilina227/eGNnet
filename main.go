package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	var (
		addr    = flag.String("addr", "127.0.0.1:8777", "listen address")
		dbPath  = flag.String("db", "", "path to SQLite database file (default: alongside the executable)")
		noOpen  = flag.Bool("no-open", false, "do not open a browser on start")
	)
	flag.Parse()

	if *dbPath == "" {
		dir := appDataDir()
		_ = os.MkdirAll(dir, 0o755)
		*dbPath = filepath.Join(dir, "eGN.db")
	}

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := db.migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	srv := newServer(db)
	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv.routes(),
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	url := "http://" + *addr + "/"
	log.Printf("eGN running at %s  (database: %s)", url, *dbPath)
	if !*noOpen {
		go func() { time.Sleep(600 * time.Millisecond); openBrowser(url) }()
	}
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// appDataDir returns a per-user writable folder for the database and files.
func appDataDir() string {
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "eGN")
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", "eGN")
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".egn")
	}
	return "."
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd, args = "open", []string{url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		fmt.Printf("Open %s in your browser.\n", url)
	}
}
