package main

import "net/http"

// Build-time metadata. These values are overridden at compile time via
// -ldflags "-X main.appEnv=prod -X main.version=1.2.3 -X main.commit=abc1234".
//
// appEnv distinguishes a development build (produced from the `dev` branch) from
// a production build (produced from the `main` branch). It is the hook for the
// future automated-update mechanism: dev builds can check a dev release channel
// and prod builds a prod channel, so officers' machines only ever pull updates
// from the channel that matches the build they are running.
//
// The defaults below apply to a plain `go build` with no ldflags (local dev).
var (
	appEnv  = "dev"
	version = "0.1.0-dev"
	commit  = "unknown"
)

// handleVersion reports the build environment and version. It is public and
// unauthenticated so the future update mechanism can query which channel and
// version a machine is running without a login.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"env":     appEnv,
		"version": version,
		"commit":  commit,
	})
}
