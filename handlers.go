package main

import (
	"database/sql"
	"net/http"
	"strings"
)

// queryMaps runs a query and returns rows as []map[string]any (NULL -> nil).
func (db *DB) queryMaps(q string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// divisionIDsForUser returns the GN division IDs a user may see.
func (s *Server) divisionIDsForUser(userID string) []string {
	rows, err := s.db.Query(`SELECT a.gn_division_id FROM officer_assignment a
		JOIN app_user u ON u.officer_id=a.officer_id
		WHERE u.id=? AND a.is_current=1 AND a.deleted_at IS NULL`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// GET /api/provinces
func (s *Server) handleProvinces(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.queryMaps(`SELECT id,code,name_en,name_si,name_ta FROM province ORDER BY name_en`)
	writeJSON(w, 200, rows)
}

// GET /api/districts?province=ID
func (s *Server) handleDistricts(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.queryMaps(`SELECT id,province_id,code,name_en,name_si,name_ta FROM district WHERE province_id=? ORDER BY name_en`,
		r.URL.Query().Get("province"))
	writeJSON(w, 200, rows)
}

// GET /api/ds-divisions?district=ID
func (s *Server) handleDSDivisions(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.queryMaps(`SELECT id,district_id,code,name_en,name_si,name_ta FROM ds_division WHERE district_id=? ORDER BY name_en`,
		r.URL.Query().Get("district"))
	writeJSON(w, 200, rows)
}

// GET /api/gn-divisions?ds=ID
func (s *Server) handleGNDivisions(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.queryMaps(`SELECT id,ds_division_id,gn_code,name_en,name_si,name_ta FROM gn_division WHERE ds_division_id=? ORDER BY gn_code`,
		r.URL.Query().Get("ds"))
	writeJSON(w, 200, rows)
}

// GET /api/my-divisions — the current user's assigned divisions (with names).
func (s *Server) handleMyDivisions(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	rows, _ := s.db.queryMaps(`SELECT g.id,g.gn_code,g.name_en,g.name_si,g.name_ta, d.name_en AS ds_name
		FROM officer_assignment a
		JOIN app_user u ON u.officer_id=a.officer_id
		JOIN gn_division g ON g.id=a.gn_division_id
		JOIN ds_division d ON d.id=g.ds_division_id
		WHERE u.id=? AND a.is_current=1 AND a.deleted_at IS NULL ORDER BY g.gn_code`, sess.userID)
	writeJSON(w, 200, rows)
}

// GET /api/lookups — all option lists, labelled in ?lang (default en).
func (s *Server) handleLookups(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	nameCol := "name_en"
	if lang == "si" {
		nameCol = "COALESCE(NULLIF(name_si,''),name_en)"
	} else if lang == "ta" {
		nameCol = "COALESCE(NULLIF(name_ta,''),name_en)"
	}
	rows, err := s.db.queryMaps(`SELECT category, code, `+nameCol+` AS name FROM lookup_option
		WHERE active=1 ORDER BY category, sort_order`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	grouped := map[string][]map[string]any{}
	for _, row := range rows {
		cat, _ := row["category"].(string)
		grouped[cat] = append(grouped[cat], map[string]any{"code": row["code"], "name": row["name"]})
	}
	writeJSON(w, 200, grouped)
}

// GET /api/welfare-programs
func (s *Server) handleWelfarePrograms(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.db.queryMaps(`SELECT id,code,name_en,name_si,name_ta,active FROM welfare_program
		WHERE deleted_at IS NULL ORDER BY name_en`)
	writeJSON(w, 200, rows)
}

// GET /api/stats — dashboard counts, scoped to the user's divisions.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	divs := s.divisionIDsForUser(sess.userID)
	if len(divs) == 0 {
		writeJSON(w, 200, map[string]any{"households": 0, "members": 0, "divisions": 0})
		return
	}
	ph := placeholders(len(divs))
	var households, members int
	s.db.QueryRow(`SELECT COUNT(*) FROM household WHERE deleted_at IS NULL AND gn_division_id IN (`+ph+`)`, toAny(divs)...).Scan(&households)
	s.db.QueryRow(`SELECT COUNT(*) FROM member m JOIN household h ON h.id=m.household_id
		WHERE m.deleted_at IS NULL AND h.deleted_at IS NULL AND h.gn_division_id IN (`+ph+`)`, toAny(divs)...).Scan(&members)
	writeJSON(w, 200, map[string]any{"households": households, "members": members, "divisions": len(divs)})
}

// GET /api/households?q=&division=&village=
func (s *Server) handleHouseholdList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	divs := s.divisionIDsForUser(sess.userID)
	if len(divs) == 0 {
		writeJSON(w, 200, []any{})
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	division := r.URL.Query().Get("division")
	village := strings.TrimSpace(r.URL.Query().Get("village"))

	where := []string{"h.deleted_at IS NULL"}
	args := []any{}
	if division != "" {
		where = append(where, "h.gn_division_id=?")
		args = append(args, division)
	} else {
		where = append(where, "h.gn_division_id IN ("+placeholders(len(divs))+")")
		args = append(args, toAny(divs)...)
	}
	if village != "" {
		where = append(where, "h.village LIKE ?")
		args = append(args, "%"+village+"%")
	}
	if q != "" {
		where = append(where, `(h.head_name LIKE ? OR h.household_number LIKE ? OR h.id IN
			(SELECT household_id FROM member WHERE deleted_at IS NULL AND (full_name_en LIKE ? OR full_name_si LIKE ? OR nic LIKE ?)))`)
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like)
	}
	sqlStr := `SELECT h.id, h.household_number, h.head_name, h.village, h.phone_mobile,
		g.name_en AS division_name, g.gn_code,
		(SELECT COUNT(*) FROM member m WHERE m.household_id=h.id AND m.deleted_at IS NULL) AS member_count
		FROM household h JOIN gn_division g ON g.id=h.gn_division_id
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY h.updated_at DESC LIMIT 500`
	rows, err := s.db.queryMaps(sqlStr, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	writeJSON(w, 200, rows)
}

// GET /api/households/{id}
func (s *Server) handleHouseholdGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	hh, err := s.db.queryMaps(`SELECT * FROM household WHERE id=? AND deleted_at IS NULL`, id)
	if err != nil || len(hh) == 0 {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	housing, _ := s.db.queryMaps(`SELECT * FROM housing WHERE household_id=? AND deleted_at IS NULL`, id)
	members, _ := s.db.queryMaps(`SELECT * FROM member WHERE household_id=? AND deleted_at IS NULL ORDER BY serial_no, is_head DESC`, id)
	welfare, _ := s.db.queryMaps(`SELECT we.*, p.name_en AS program_name FROM welfare_enrolment we
		JOIN welfare_program p ON p.id=we.program_id WHERE we.household_id=? AND we.deleted_at IS NULL`, id)
	var housingRow any
	if len(housing) > 0 {
		housingRow = housing[0]
	}
	writeJSON(w, 200, map[string]any{
		"household": hh[0], "housing": housingRow,
		"members": orEmpty(members), "welfare": orEmpty(welfare),
	})
}

func orEmpty(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}

// POST /api/households
func (s *Server) handleHouseholdCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	div, _ := body["gn_division_id"].(string)
	if div == "" {
		writeJSON(w, 400, map[string]string{"error": "gn_division_id is required"})
		return
	}
	if !contains(s.divisionIDsForUser(sess.userID), div) {
		writeJSON(w, 403, map[string]string{"error": "division not assigned to you"})
		return
	}
	id := newID()
	now := nowISO()
	cols := []string{"id", "gn_division_id", "created_at", "updated_at", "surveyed_by"}
	vals := []any{id, div, now, now, officerOf(s, sess.userID)}
	for _, f := range householdFields {
		if v, ok := body[f]; ok {
			cols = append(cols, f)
			vals = append(vals, v)
		}
	}
	q := `INSERT INTO household(` + strings.Join(cols, ",") + `) VALUES(` + placeholders(len(cols)) + `)`
	if _, err := s.db.Exec(q, vals...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.Exec(`INSERT INTO housing(id,household_id,created_at,updated_at) VALUES(?,?,?,?)`, newID(), id, now, now)
	upsertHousing(s.db, id, body, now)
	s.db.audit(sess.userID, "household", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

// PUT /api/households/{id}
func (s *Server) handleHouseholdUpdate(w http.ResponseWriter, r *http.Request) {
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
	for _, f := range householdFields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE household SET `+strings.Join(set, ",")+` WHERE id=? AND deleted_at IS NULL`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// ensure housing row exists, then update
	s.db.Exec(`INSERT OR IGNORE INTO housing(id,household_id,created_at,updated_at) VALUES(?,?,?,?)`, newID(), id, now, now)
	upsertHousing(s.db, id, body, now)
	s.db.audit(sess.userID, "household", id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/households/{id}
func (s *Server) handleHouseholdDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	now := nowISO()
	s.db.Exec(`UPDATE household SET deleted_at=? WHERE id=?`, now, id)
	s.db.audit(sess.userID, "household", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func officerOf(s *Server, userID string) any {
	var oid sql.NullString
	s.db.QueryRow(`SELECT officer_id FROM app_user WHERE id=?`, userID).Scan(&oid)
	if oid.Valid {
		return oid.String
	}
	return nil
}

var householdFields = []string{
	"household_number", "village", "village_part", "road", "post_office", "address",
	"head_name", "phone_home", "phone_mobile", "members_female", "members_male", "members_total",
	"family_units", "living_arrangement", "shared_with", "gps_lat", "gps_lng",
	"file_name", "file_code", "file_number", "survey_date",
}

var housingFields = []string{
	"plot_ownership", "deed_no", "lot_no", "plot_acres", "plot_roods", "plot_perches",
	"house_length", "house_width", "floor_area", "room_count", "year_built", "year_occupied",
	"house_ownership", "house_type", "wall_type", "roof_type", "floor_type", "toilet_type",
	"drinking_water", "other_water", "has_electricity", "monthly_electricity_bill", "no_power_alternative",
	"cooking_fuel", "telephone_fixed", "mobile_normal", "mobile_smart", "access_road", "vehicle_access", "garbage_disposal",
}

func upsertHousing(db *DB, householdID string, body map[string]any, now string) {
	housing, ok := body["housing"].(map[string]any)
	if !ok {
		return
	}
	set := []string{"updated_at=?"}
	args := []any{now}
	for _, f := range housingFields {
		if v, ok := housing[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	if len(set) == 1 {
		return
	}
	args = append(args, householdID)
	db.Exec(`UPDATE housing SET `+strings.Join(set, ",")+` WHERE household_id=?`, args...)
}
