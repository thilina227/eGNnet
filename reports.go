package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

// Reports are rendered server-side as clean, self-contained printable HTML
// pages (A4, print stylesheet). The officer prints them or uses the browser's
// "Save as PDF" — which shapes Sinhala/Tamil correctly and keeps the app a
// single dependency-free binary (no bundled PDF/Chromium engine). Each report
// is scoped to the user's assigned divisions.

// reportLabels loads the embedded i18n map for a language (falls back to en).
func reportLabels(lang string) map[string]string {
	if lang != "si" && lang != "ta" {
		lang = "en"
	}
	b, err := webFS.ReadFile("internal/web/i18n/" + lang + ".json")
	if err != nil {
		b, _ = webFS.ReadFile("internal/web/i18n/en.json")
	}
	m := map[string]string{}
	_ = json.Unmarshal(b, &m)
	return m
}

// lookupLabels returns a code->label map for every lookup category in the lang.
func (s *Server) lookupLabels(lang string) map[string]string {
	col := "name_en"
	if lang == "si" {
		col = "COALESCE(NULLIF(name_si,''),name_en)"
	} else if lang == "ta" {
		col = "COALESCE(NULLIF(name_ta,''),name_en)"
	}
	rows, _ := s.db.queryMaps(`SELECT category, code, ` + col + ` AS name FROM lookup_option`)
	m := map[string]string{}
	for _, r := range rows {
		cat, _ := r["category"].(string)
		code, _ := r["code"].(string)
		name, _ := r["name"].(string)
		m[cat+"|"+code] = name
	}
	return m
}

func (s *Server) reportLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "en" || l == "si" || l == "ta" {
		return l
	}
	var lang string
	_ = s.db.QueryRow(`SELECT default_language FROM app_config WHERE id=1`).Scan(&lang)
	if lang == "" {
		lang = "en"
	}
	return lang
}

func (s *Server) officeName() string {
	var n string
	_ = s.db.QueryRow(`SELECT COALESCE(office_name,'') FROM app_config WHERE id=1`).Scan(&n)
	if n == "" {
		n = "eGNnet"
	}
	return n
}

// reportPage wraps a report body in a standalone printable HTML document.
func reportPage(title, office, body string) string {
	return `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: system-ui, "Noto Sans", "Noto Sans Sinhala", "Noto Sans Tamil", sans-serif; color:#1c2420; margin:0; padding:24px; background:#f4f6f5; }
  .sheet { background:#fff; max-width:800px; margin:0 auto; padding:32px 36px; box-shadow:0 1px 6px rgba(0,0,0,.1); }
  .rpt-head { display:flex; justify-content:space-between; align-items:flex-start; border-bottom:2px solid #0f6e63; padding-bottom:10px; margin-bottom:18px; }
  .rpt-head .office { font-size:1.3rem; font-weight:700; color:#0f6e63; }
  .rpt-head .meta { text-align:right; font-size:.8rem; color:#5a655e; }
  h1 { font-size:1.1rem; margin:0 0 4px; }
  h2 { font-size:.95rem; color:#0b574e; border-bottom:1px solid #e2e8e4; padding-bottom:4px; margin:22px 0 10px; }
  table { width:100%; border-collapse:collapse; font-size:.82rem; margin:6px 0; }
  th, td { text-align:left; padding:5px 8px; border-bottom:1px solid #eef2f0; vertical-align:top; }
  th { color:#5a655e; font-weight:600; background:#f7faf9; }
  .kv { display:grid; grid-template-columns:repeat(2,1fr); gap:4px 24px; font-size:.85rem; }
  .kv div { padding:3px 0; border-bottom:1px solid #f1f5f3; }
  .kv b { color:#5a655e; font-weight:600; display:inline-block; min-width:140px; }
  .muted { color:#5a655e; }
  .toolbar { max-width:800px; margin:0 auto 14px; display:flex; gap:10px; }
  .btn { background:#0f6e63; color:#fff; border:0; border-radius:8px; padding:9px 16px; font-size:.9rem; font-weight:600; cursor:pointer; text-decoration:none; }
  .empty { color:#9aa5a0; }
  @media print {
    body { background:#fff; padding:0; }
    .sheet { box-shadow:none; max-width:none; padding:0; }
    .toolbar { display:none; }
    @page { size:A4; margin:16mm; }
  }
</style></head><body>
<div class="toolbar"><button class="btn" onclick="window.print()">` + "Print / Save as PDF" + `</button></div>
<div class="sheet">
  <div class="rpt-head"><div class="office">` + html.EscapeString(office) + `</div>
    <div class="meta">` + html.EscapeString(title) + `<br>` + time.Now().Format("2006-01-02 15:04") + `</div></div>
  ` + body + `
</div></body></html>`
}

func esc(s any) string {
	if s == nil {
		return ""
	}
	return html.EscapeString(fmt.Sprintf("%v", s))
}

func writeHTML(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(s))
}

// GET /report/household/{id}
func (s *Server) handleHouseholdReport(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	hh, err := s.db.queryMaps(`SELECT * FROM household WHERE id=? AND deleted_at IS NULL`, id)
	if err != nil || len(hh) == 0 {
		http.Error(w, "not found", 404)
		return
	}
	H := hh[0]
	div, _ := H["gn_division_id"].(string)
	if !contains(s.divisionIDsForUser(sess.userID), div) {
		http.Error(w, "forbidden", 403)
		return
	}
	lang := s.reportLang(r)
	L := reportLabels(lang)
	lk := s.lookupLabels(lang)
	tr := func(k string) string {
		if v, ok := L[k]; ok {
			return v
		}
		return k
	}
	lab := func(cat, code string) string {
		if code == "" {
			return ""
		}
		if v, ok := lk[cat+"|"+code]; ok {
			return v
		}
		return code
	}

	var b strings.Builder
	divName := ""
	if drow, _ := s.db.queryMaps(`SELECT gn_code, name_en, name_si, name_ta FROM gn_division WHERE id=?`, div); len(drow) > 0 {
		divName = esc(drow[0]["gn_code"]) + " · " + esc(drow[0]["name_"+lang])
		if divName == " · " {
			divName = esc(drow[0]["name_en"])
		}
	}
	fmt.Fprintf(&b, `<h1>%s</h1>`, tr("household_info"))
	b.WriteString(`<div class="kv">`)
	kv := func(k, v string) {
		fmt.Fprintf(&b, `<div><b>%s</b>%s</div>`, html.EscapeString(k), esc(v))
	}
	kv(tr("household_number"), fmt.Sprint(orNil(H["household_number"])))
	kv(tr("head_name"), fmt.Sprint(orNil(H["head_name"])))
	kv(tr("village"), fmt.Sprint(orNil(H["village"])))
	kv(tr("address"), fmt.Sprint(orNil(H["address"])))
	kv(tr("phone_mobile"), fmt.Sprint(orNil(H["phone_mobile"])))
	kv(tr("phone_home"), fmt.Sprint(orNil(H["phone_home"])))
	b.WriteString(`</div>`)
	fmt.Fprintf(&b, `<p class="muted">%s: %s</p>`, html.EscapeString(tr("gn_division")), divName)

	// Housing
	if hs, _ := s.db.queryMaps(`SELECT * FROM housing WHERE household_id=? AND deleted_at IS NULL`, id); len(hs) > 0 {
		HS := hs[0]
		fmt.Fprintf(&b, `<h2>%s</h2><div class="kv">`, tr("housing_info"))
		kv(tr("house_type"), lab("house_type", fmt.Sprint(orNil(HS["house_type"]))))
		kv(tr("wall_type"), lab("wall_type", fmt.Sprint(orNil(HS["wall_type"]))))
		kv(tr("roof_type"), lab("roof_type", fmt.Sprint(orNil(HS["roof_type"]))))
		kv(tr("floor_type"), lab("floor_type", fmt.Sprint(orNil(HS["floor_type"]))))
		kv(tr("toilet_type"), lab("toilet_type", fmt.Sprint(orNil(HS["toilet_type"]))))
		kv(tr("drinking_water"), lab("water_source", fmt.Sprint(orNil(HS["drinking_water"]))))
		kv(tr("cooking_fuel"), lab("cooking_fuel", fmt.Sprint(orNil(HS["cooking_fuel"]))))
		elec := tr("no")
		if fmt.Sprint(orNil(HS["has_electricity"])) == "1" {
			elec = tr("yes")
		}
		kv(tr("has_electricity"), elec)
		b.WriteString(`</div>`)
	}

	// Members
	members, _ := s.db.queryMaps(`SELECT * FROM member WHERE household_id=? AND deleted_at IS NULL ORDER BY serial_no, is_head DESC`, id)
	fmt.Fprintf(&b, `<h2>%s (%d)</h2>`, tr("members_in_household"), len(members))
	if len(members) > 0 {
		fmt.Fprintf(&b, `<table><thead><tr><th>%s</th><th>%s</th><th>%s</th><th>%s</th><th>%s</th><th>%s</th></tr></thead><tbody>`,
			tr("full_name"), tr("relationship"), tr("nic"), tr("gender"), tr("dob"), tr("occupation"))
		for _, m := range members {
			name := fmt.Sprint(orNil(m["full_name_en"]))
			if name == "" {
				name = fmt.Sprint(orNil(m["full_name_si"]))
			}
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				esc(name), esc(lab("relationship", fmt.Sprint(orNil(m["relationship_to_head"])))),
				esc(orNil(m["nic"])), esc(lab("gender", fmt.Sprint(orNil(m["gender"])))),
				esc(orNil(m["dob"])), esc(orNil(m["occupation"])))
		}
		b.WriteString(`</tbody></table>`)
	} else {
		fmt.Fprintf(&b, `<p class="empty">—</p>`)
	}

	// Welfare
	welfare, _ := s.db.queryMaps(`SELECT we.*, p.name_en AS program_name FROM welfare_enrolment we
		JOIN welfare_program p ON p.id=we.program_id WHERE we.household_id=? AND we.deleted_at IS NULL`, id)
	if len(welfare) > 0 {
		fmt.Fprintf(&b, `<h2>%s</h2><table><thead><tr><th>%s</th><th>%s</th></tr></thead><tbody>`,
			tr("welfare"), tr("program"), tr("amount"))
		for _, we := range welfare {
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td></tr>`, esc(orNil(we["program_name"])), esc(orNil(we["amount"])))
		}
		b.WriteString(`</tbody></table>`)
	}

	// Assets snapshot (land, livestock, vehicles) + income bracket
	assetBlock(&b, s, id, tr, lab, lang)

	writeHTML(w, reportPage(s.officeName()+" — "+tr("household_info"), s.officeName(), b.String()))
}

func assetBlock(b *strings.Builder, s *Server, hhID string, tr func(string) string, lab func(string, string) string, lang string) {
	land, _ := s.db.queryMaps(`SELECT * FROM land_holding WHERE household_id=? AND deleted_at IS NULL`, hhID)
	live, _ := s.db.queryMaps(`SELECT * FROM livestock WHERE household_id=? AND deleted_at IS NULL`, hhID)
	veh, _ := s.db.queryMaps(`SELECT * FROM vehicle WHERE household_id=? AND deleted_at IS NULL`, hhID)
	fin, _ := s.db.queryMaps(`SELECT * FROM household_finance WHERE household_id=? AND deleted_at IS NULL`, hhID)
	if len(land)+len(live)+len(veh)+len(fin) == 0 {
		return
	}
	fmt.Fprintf(b, `<h2>%s</h2>`, tr("assets_economy"))
	if len(land) > 0 {
		fmt.Fprintf(b, `<table><thead><tr><th>%s</th><th>%s</th><th>%s</th></tr></thead><tbody>`, tr("land_holdings"), tr("land_class"), tr("acres"))
		for _, l := range land {
			fmt.Fprintf(b, `<tr><td>%s</td><td>%s</td><td>%s</td></tr>`, esc(lab("land_class", fmt.Sprint(orNil(l["land_class"])))), esc(lab("land_tenure", fmt.Sprint(orNil(l["tenure_type"])))), esc(orNil(l["acres"])))
		}
		b.WriteString(`</tbody></table>`)
	}
	if len(live) > 0 {
		fmt.Fprintf(b, `<table><thead><tr><th>%s</th><th>%s</th></tr></thead><tbody>`, tr("animal_type"), tr("count"))
		for _, l := range live {
			fmt.Fprintf(b, `<tr><td>%s</td><td>%s</td></tr>`, esc(lab("livestock_type", fmt.Sprint(orNil(l["animal_type"])))), esc(orNil(l["count"])))
		}
		b.WriteString(`</tbody></table>`)
	}
	if len(veh) > 0 {
		fmt.Fprintf(b, `<table><thead><tr><th>%s</th><th>%s</th></tr></thead><tbody>`, tr("vehicle_type"), tr("registration_no"))
		for _, l := range veh {
			fmt.Fprintf(b, `<tr><td>%s</td><td>%s</td></tr>`, esc(lab("vehicle_type", fmt.Sprint(orNil(l["vehicle_type"])))), esc(orNil(l["registration_no"])))
		}
		b.WriteString(`</tbody></table>`)
	}
	if len(fin) > 0 {
		fmt.Fprintf(b, `<p><b>%s:</b> %s</p>`, html.EscapeString(tr("monthly_income_bracket")), esc(lab("income_bracket", fmt.Sprint(orNil(fin[0]["monthly_income_bracket"])))))
	}
}

// GET /report/division/{id}
func (s *Server) handleDivisionReport(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if !contains(s.divisionIDsForUser(sess.userID), id) {
		http.Error(w, "forbidden", 403)
		return
	}
	lang := s.reportLang(r)
	L := reportLabels(lang)
	tr := func(k string) string {
		if v, ok := L[k]; ok {
			return v
		}
		return k
	}

	drow, _ := s.db.queryMaps(`SELECT g.gn_code, g.name_en, g.name_si, g.name_ta, d.name_en AS ds_name
		FROM gn_division g JOIN ds_division d ON d.id=g.ds_division_id WHERE g.id=?`, id)
	if len(drow) == 0 {
		http.Error(w, "not found", 404)
		return
	}
	D := drow[0]
	var hhCount, memCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM household WHERE gn_division_id=? AND deleted_at IS NULL`, id).Scan(&hhCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM member m JOIN household h ON h.id=m.household_id
		WHERE h.gn_division_id=? AND h.deleted_at IS NULL AND m.deleted_at IS NULL`, id).Scan(&memCount)

	var b strings.Builder
	divName := esc(D["gn_code"]) + " · " + esc(D["name_"+lang])
	fmt.Fprintf(&b, `<h1>%s — %s</h1><p class="muted">%s: %s</p>`, tr("gn_division"), divName, tr("ds_division"), esc(D["ds_name"]))
	fmt.Fprintf(&b, `<div class="kv"><div><b>%s</b>%d</div><div><b>%s</b>%d</div></div>`,
		html.EscapeString(tr("households")), hhCount, html.EscapeString(tr("members")), memCount)

	hhs, _ := s.db.queryMaps(`SELECT h.household_number, h.head_name, h.village,
		(SELECT COUNT(*) FROM member m WHERE m.household_id=h.id AND m.deleted_at IS NULL) AS mc
		FROM household h WHERE h.gn_division_id=? AND h.deleted_at IS NULL ORDER BY h.household_number LIMIT 2000`, id)
	fmt.Fprintf(&b, `<h2>%s (%d)</h2>`, tr("households"), len(hhs))
	if len(hhs) > 0 {
		fmt.Fprintf(&b, `<table><thead><tr><th>%s</th><th>%s</th><th>%s</th><th>%s</th></tr></thead><tbody>`,
			tr("household_number"), tr("head_name"), tr("village"), tr("member_count"))
		for _, h := range hhs {
			fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				esc(orNil(h["household_number"])), esc(orNil(h["head_name"])), esc(orNil(h["village"])), esc(orNil(h["mc"])))
		}
		b.WriteString(`</tbody></table>`)
	} else {
		fmt.Fprintf(&b, `<p class="empty">%s</p>`, tr("no_results"))
	}
	writeHTML(w, reportPage(s.officeName()+" — "+tr("gn_division"), s.officeName(), b.String()))
}

func orNil(v any) any {
	if v == nil {
		return ""
	}
	return v
}
