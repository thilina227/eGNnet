package main

import (
	"net/http"
	"strings"
)

// assetCollection describes a household child-table that the asset/economy
// screens manage. The table name is a fixed whitelist value (never user input),
// so it is safe to interpolate into SQL; only whitelisted fields are accepted.
type assetCollection struct {
	table  string
	fields []string
}

var assetCollections = map[string]assetCollection{
	"land_holding":      {"land_holding", []string{"land_class", "tenure_type", "acres", "roods", "perches", "is_main", "notes"}},
	"land_crop":         {"land_crop", []string{"crop_type", "extent_or_trees", "is_main"}},
	"paddy_cultivation": {"paddy_cultivation", []string{"season", "extent", "method"}},
	"livestock":         {"livestock", []string{"animal_type", "count"}},
	"poultry":           {"poultry", []string{"poultry_type", "count"}},
	"appliance":         {"appliance", []string{"appliance_type", "count"}},
	"vehicle":           {"vehicle", []string{"vehicle_type", "registration_no", "count", "income_use"}},
	"equipment":         {"equipment", []string{"equipment_type", "count", "income_use"}},
	"permit":            {"permit", []string{"member_id", "permit_type", "permit_no", "holder_name", "valid_until"}},
	"loan":              {"loan", []string{"is_microfinance", "provider", "details"}},
	"state_aid":         {"state_aid", []string{"project", "year", "aid_item", "progress"}},
	"income_source":     {"income_source", []string{"source_type"}},
	"monthly_expense":   {"monthly_expense", []string{"category", "amount"}},
}

// assetOrder gives a stable order for the combined GET response.
var assetOrder = []string{
	"land_holding", "land_crop", "paddy_cultivation",
	"livestock", "poultry", "appliance", "vehicle", "equipment", "permit",
	"loan", "state_aid", "income_source", "monthly_expense",
}

// coerce converts JSON values into SQLite-friendly ones (bool -> 0/1).
func coerce(v any) any {
	switch b := v.(type) {
	case bool:
		if b {
			return 1
		}
		return 0
	default:
		return v
	}
}

// GET /api/households/{id}/assets — every asset/economy collection plus the
// 1:1 household_finance row, in one payload for the detail screen.
func (s *Server) handleAssetsGet(w http.ResponseWriter, r *http.Request) {
	hid := r.PathValue("id")
	out := map[string]any{}
	for _, name := range assetOrder {
		def := assetCollections[name]
		rows, _ := s.db.queryMaps(`SELECT * FROM `+def.table+` WHERE household_id=? AND deleted_at IS NULL ORDER BY created_at`, hid)
		out[name] = orEmpty(rows)
	}
	fin, _ := s.db.queryMaps(`SELECT * FROM household_finance WHERE household_id=? AND deleted_at IS NULL`, hid)
	if len(fin) > 0 {
		out["household_finance"] = fin[0]
	} else {
		out["household_finance"] = nil
	}
	writeJSON(w, 200, out)
}

// POST /api/households/{id}/assets/{collection}
func (s *Server) handleAssetCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	hid := r.PathValue("id")
	coll := r.PathValue("collection")
	def, ok := assetCollections[coll]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unknown collection"})
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	id := newID()
	now := nowISO()
	cols := []string{"id", "household_id", "created_at", "updated_at"}
	vals := []any{id, hid, now, now}
	for _, f := range def.fields {
		if v, ok := body[f]; ok {
			cols = append(cols, f)
			vals = append(vals, coerce(v))
		}
	}
	q := `INSERT INTO ` + def.table + `(` + strings.Join(cols, ",") + `) VALUES(` + placeholders(len(cols)) + `)`
	if _, err := s.db.Exec(q, vals...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, def.table, id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

// PUT /api/assets/{collection}/{id}
func (s *Server) handleAssetUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	coll := r.PathValue("collection")
	id := r.PathValue("id")
	def, ok := assetCollections[coll]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unknown collection"})
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	now := nowISO()
	set := []string{"updated_at=?"}
	args := []any{now}
	for _, f := range def.fields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, coerce(v))
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE `+def.table+` SET `+strings.Join(set, ",")+` WHERE id=? AND deleted_at IS NULL`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, def.table, id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/assets/{collection}/{id}
func (s *Server) handleAssetDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	coll := r.PathValue("collection")
	id := r.PathValue("id")
	def, ok := assetCollections[coll]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unknown collection"})
		return
	}
	s.db.Exec(`UPDATE `+def.table+` SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, def.table, id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// PUT /api/households/{id}/finance — upsert the 1:1 household_finance row.
func (s *Server) handleFinanceUpsert(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	hid := r.PathValue("id")
	var body struct {
		MonthlyIncomeBracket string `json:"monthly_income_bracket"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	now := nowISO()
	s.db.Exec(`INSERT OR IGNORE INTO household_finance(id,household_id,created_at,updated_at) VALUES(?,?,?,?)`, newID(), hid, now, now)
	if _, err := s.db.Exec(`UPDATE household_finance SET monthly_income_bracket=?, updated_at=? WHERE household_id=?`,
		body.MonthlyIncomeBracket, now, hid); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "household_finance", hid, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
