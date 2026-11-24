# eGNnet — Grama Niladhari household records

A trilingual (English / Sinhala / Tamil-ready) household records system for Sri Lankan
Grama Niladhari divisions. Single self-contained executable with an embedded web server and
an embedded SQLite database — no separate web or database server, no runtime to install.

This is **v0.1**, a working vertical slice. See _Status_ below for what's implemented.

## Run it

**Windows (as shipped to officers):** double-click `eGNnet.exe`. It starts a local server and
opens the app in your default browser. All data is stored in one file under
`%LOCALAPPDATA%\eGNnet\eGNnet.db`.

**From source (any platform):**

```bash
go build -o egnnet .     # produces a single binary
./egnnet                 # starts http://127.0.0.1:8777 and opens a browser
./egnnet --no-open       # don't open a browser
./egnnet --addr 127.0.0.1:9000 --db /path/to/eGNnet.db
```

First launch shows a **setup wizard**: choose your Province → District → DS → GN division(s),
enter the officer's details, pick a language and create a login. The app then opens to the
dashboard.

## What's inside

```
main.go              entry point, flags, app-data folder, browser launch
server.go            routes, static UI, session middleware
auth.go              first-run setup, login/logout, sessions (bcrypt)
handlers.go          hierarchy, lookups, stats, household list/search/CRUD
members_welfare.go   members, member flags, welfare enrolments, i18n endpoint
db.go                SQLite open/migrate/seed, UUIDs, audit log
buildinfo.go         build-time env/version/commit; GET /api/version
internal/db/schema.sql   full schema (~34 tables)
internal/db/seed.sql     admin hierarchy + 200+ lookup options (EN+Sinhala) + welfare programs
internal/web/            embedded UI: index.html, app.js, styles.css, i18n/{en,si,ta}.json
```

- **Database:** SQLite via `modernc.org/sqlite` (pure Go — no CGO, no DLL).
- **Keys:** UUID primary keys; every table carries `created_at` / `updated_at` / `deleted_at`
  (soft delete). An `audit_log` records every change — the foundation for the future central sync.
- **Localisation:** coded option lists live in `lookup_option` with `name_en` / `name_si` /
  `name_ta`; UI chrome is in `internal/web/i18n/*.json`. Tamil columns/files are present and empty,
  ready to fill.
- **Multi-division:** officers are linked to divisions through `officer_assignment`, so one officer
  can hold several current assignments.

## Status (v0.1)

Implemented end-to-end: first-run setup, login, dashboard, household list + multi-criteria search,
household + housing capture, members (with special-category flags), welfare enrolments, and the
English/Sinhala language switch.

Schema and seed data cover the **whole** household form (land, crops, livestock, appliances,
vehicles, equipment, permits, income/expenses, state aid). The data-entry **screens** for those
asset/economy sections are the next iteration — the tables, APIs pattern and lookups are already in
place.

## Packaging for Windows (next step)

- **Native window:** wrap with [Wails](https://wails.io) so it opens as a desktop window using the
  built-in WebView2 instead of a browser tab.
- **Installer:** an [Inno Setup](https://jrsoftware.org/isinfo.php) script bundles `eGNnet.exe`, a
  pre-seeded `eGNnet.db` and shortcuts into one wizard installer.
- **Encryption (optional):** SQLCipher for the database file, or rely on Windows BitLocker.

## Cross-compile

Thanks to the pure-Go SQLite driver there is no CGO, so every target builds from any host:

```bash
# Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o eGNnet.exe .

# Linux (Ubuntu / amd64)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o egnnet .
```

On Ubuntu you can also just run `go build -o egnnet .` natively. The CI pipeline
(`.github/workflows/build.yml`) builds both the Windows and Linux binaries on every push
and uploads them as artifacts.
