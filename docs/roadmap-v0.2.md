# eGNnet — v0.2 roadmap

Planned work after v0.1. Each item below is a **distinct module**, shipped as its own PR into `dev`.
Driven by (a) the official GN household form and (b) a feature comparison against the original
Java/MySQL app — see [`architecture.md`](architecture.md). Items marked _(gap)_ existed in the old
app but are not yet in eGNnet.

## Module 1 — Asset & economy capture screens
The schema and coded lookups already exist (`land_holding`, `land_crop`, `paddy_cultivation`,
`livestock`, `poultry`, `appliance`, `vehicle`, `equipment`, `permit`, `loan`, `state_aid`,
`household_finance`, `income_source`, `monthly_expense`). This module adds the **data-entry UI**
and wires the CRUD APIs, following the existing household/members pattern.
- Each section is a panel on the household detail page (add / edit / delete rows).
- Note: the old app attached land & vehicles to a *person*; here they belong to the *household*.
- Completing this makes v0.2 a usable end-to-end capture of the whole form.

## Module 2 — Member photos _(gap)_
Old app stored `person.image` (+ `prof_pics/` folders). Add an optional photo per member.
- Store files under the app-data folder (not as DB blobs); keep a `member.photo_path` reference.
- Capture/upload in the member editor; thumbnail in the member list.

## Module 3 — Office inventory _(gap)_
Old app had `inventory` + `history` tables (shipped empty). Rebuild as a proper module.
- New tables: `inventory_item` (name, category, quantity, condition, location, division scope) and
  `inventory_txn` (item, change, reason, date, officer) for the movement/history log.
- CRUD UI + list/search; division-scoped like households.

## Module 4 — Work diary / day-book _(gap)_
Old `dairy` table — the officer's running log of daily duties.
- New table `diary_entry` (date, officer, category, note, optional linked household).
- Day view + add entry; filter by date range and officer.

## Module 5 — Transfer-of-work log _(gap)_
Old `transferwork` table. Partly overlapped by our dated `officer_assignment` history, but the full
version tracks handover of specific work/cases.
- New table `work_transfer` (from_officer, to_officer, subject, handed_over_at, status, notes).
- Keep lightweight; most office-transfer context already lives in `officer_assignment`.

## Module 6 — Reports & PDF _(gap)_
Old app produced printable reports (`reports` table held paths).
- Generate printable/PDF household profiles, member lists, and division summaries.
- Server-side render (HTML template → PDF) so it works headless and in the installer build.

## Module 7 — Application log, with archival & retention _(new requirement)_
Distinct from the existing **`audit_log`** (which is the per-row data-change trail). This is an
**operational application log**: logins/logouts, errors, sync events, exports, administrative
actions.
- **Where:** structured entries to a rolling log file in the app-data folder, and/or an `app_log`
  table for in-app viewing. Decide one source of truth; file + periodic import is fine.
- **Archival:** rotate on size/date (e.g. daily), compress and move older files to an `archive/`
  subfolder of the app-data folder.
- **Retention:** a configurable retention window in `app_config` (e.g. keep 90 days online, archive
  up to N months, then purge). A scheduled sweep on startup/interval enforces it. Apply the same
  retention/archival policy to `audit_log` so the DB doesn't grow unbounded.
- **Admin UI:** a settings screen to view recent logs and set the retention window; export a range.
- Keep it privacy-aware: log actors and actions, not citizen PII payloads.

## Already covered in v0.1 (for reference)
Dashboard (division-scoped stats), household + housing + members (full-census, richer than the old
app's ~⅓-of-form), configurable welfare programs/enrolments, multi-criteria citizen search, roles +
multi-division switching, `audit_log` on writes, EN/Sinhala i18n.

## Suggested sequence
1. **Module 1** (asset/economy screens) — biggest value, makes the form end-to-end.
2. **Module 7** (application log + retention) — cross-cutting; best in early so later modules log through it.
3. **Module 2** (member photos).
4. **Module 3** (inventory), **Module 4** (work diary), **Module 5** (transfer log).
5. **Module 6** (reports & PDF).

Each module lands as its own PR to `dev`; promotions to `main` cut releases as usual.
