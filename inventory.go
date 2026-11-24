package main

import (
	"net/http"
	"strings"
)

// Office inventory: a division-scoped stock register (inventory_item) with a
// movement history (inventory_txn). Item balances change only through
// movements, so the running quantity and the history stay consistent.

// itemDivisionVisible reports whether the item exists and sits in a division
// the user may see; it also returns the item's current balance.
func (s *Server) itemDivisionVisible(userID, itemID string) (div string, qty float64, ok bool) {
	err := s.db.QueryRow(`SELECT gn_division_id, quantity FROM inventory_item WHERE id=? AND deleted_at IS NULL`, itemID).Scan(&div, &qty)
	if err != nil {
		return "", 0, false
	}
	return div, qty, contains(s.divisionIDsForUser(userID), div)
}

// GET /api/inventory?division=&q=
func (s *Server) handleInventoryList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	divs := s.divisionIDsForUser(sess.userID)
	if len(divs) == 0 {
		writeJSON(w, 200, []any{})
		return
	}
	where := []string{"i.deleted_at IS NULL"}
	args := []any{}
	if division := r.URL.Query().Get("division"); division != "" && contains(divs, division) {
		where = append(where, "i.gn_division_id=?")
		args = append(args, division)
	} else {
		where = append(where, "i.gn_division_id IN ("+placeholders(len(divs))+")")
		args = append(args, toAny(divs)...)
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		where = append(where, "(i.name LIKE ? OR i.category LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	rows, err := s.db.queryMaps(`SELECT i.id, i.gn_division_id, i.name, i.category, i.unit,
		i.quantity, i.min_quantity, i.notes, g.name_en AS division_name, g.gn_code,
		(SELECT COUNT(*) FROM inventory_txn t WHERE t.item_id=i.id) AS txn_count
		FROM inventory_item i JOIN gn_division g ON g.id=i.gn_division_id
		WHERE `+strings.Join(where, " AND ")+` ORDER BY i.name LIMIT 1000`, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/inventory  {gn_division_id, name, category, unit, min_quantity, notes, opening_quantity}
func (s *Server) handleInventoryCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var body struct {
		DivisionID      string   `json:"gn_division_id"`
		Name            string   `json:"name"`
		Category        string   `json:"category"`
		Unit            string   `json:"unit"`
		MinQuantity     *float64 `json:"min_quantity"`
		Notes           string   `json:"notes"`
		OpeningQuantity float64  `json:"opening_quantity"`
	}
	if err := readJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" || body.DivisionID == "" {
		writeJSON(w, 400, map[string]string{"error": "name and gn_division_id are required"})
		return
	}
	if !contains(s.divisionIDsForUser(sess.userID), body.DivisionID) {
		writeJSON(w, 403, map[string]string{"error": "division not assigned to you"})
		return
	}
	id := newID()
	now := nowISO()
	open := body.OpeningQuantity
	if open < 0 {
		open = 0
	}
	if _, err := s.db.Exec(`INSERT INTO inventory_item(id,gn_division_id,name,category,unit,quantity,min_quantity,notes,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, id, body.DivisionID, strings.TrimSpace(body.Name), body.Category, body.Unit, open, body.MinQuantity, body.Notes, now, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if open > 0 {
		s.db.Exec(`INSERT INTO inventory_txn(id,item_id,txn_type,quantity,balance_after,reference,note,user_id,txn_date,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?)`, newID(), id, "in", open, open, "", "opening balance", sess.userID, now[:10], now)
	}
	s.db.audit(sess.userID, "inventory_item", id, "create", nil)
	writeJSON(w, 200, map[string]any{"id": id})
}

var inventoryItemFields = []string{"name", "category", "unit", "min_quantity", "notes"}

// PUT /api/inventory/{id} — edit item metadata (not the balance; that moves via txns).
func (s *Server) handleInventoryUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if _, _, ok := s.itemDivisionVisible(sess.userID, id); !ok {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	var body map[string]any
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	set := []string{"updated_at=?"}
	args := []any{nowISO()}
	for _, f := range inventoryItemFields {
		if v, ok := body[f]; ok {
			set = append(set, f+"=?")
			args = append(args, v)
		}
	}
	args = append(args, id)
	if _, err := s.db.Exec(`UPDATE inventory_item SET `+strings.Join(set, ",")+` WHERE id=?`, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "inventory_item", id, "update", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/inventory/{id}
func (s *Server) handleInventoryDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if _, _, ok := s.itemDivisionVisible(sess.userID, id); !ok {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	s.db.Exec(`UPDATE inventory_item SET deleted_at=? WHERE id=?`, nowISO(), id)
	s.db.audit(sess.userID, "inventory_item", id, "delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/inventory/{id}/txns — movement history, newest first.
func (s *Server) handleInventoryTxns(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	if _, _, ok := s.itemDivisionVisible(sess.userID, id); !ok {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	rows, _ := s.db.queryMaps(`SELECT id,txn_type,quantity,balance_after,reference,note,txn_date,created_at
		FROM inventory_txn WHERE item_id=? ORDER BY created_at DESC LIMIT 1000`, id)
	writeJSON(w, 200, orEmpty(rows))
}

// POST /api/inventory/{id}/txns  {txn_type, quantity, reference, note, txn_date}
func (s *Server) handleInventoryTxnCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	id := r.PathValue("id")
	_, bal, ok := s.itemDivisionVisible(sess.userID, id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	var body struct {
		TxnType   string  `json:"txn_type"`
		Quantity  float64 `json:"quantity"`
		Reference string  `json:"reference"`
		Note      string  `json:"note"`
		TxnDate   string  `json:"txn_date"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad request"})
		return
	}
	if body.Quantity <= 0 {
		writeJSON(w, 400, map[string]string{"error": "quantity must be greater than zero"})
		return
	}
	var newBal float64
	switch body.TxnType {
	case "in":
		newBal = bal + body.Quantity
	case "out":
		if body.Quantity > bal {
			writeJSON(w, 400, map[string]string{"error": "insufficient stock"})
			return
		}
		newBal = bal - body.Quantity
	case "adjust":
		newBal = body.Quantity // set absolute balance
	default:
		writeJSON(w, 400, map[string]string{"error": "txn_type must be in, out or adjust"})
		return
	}
	now := nowISO()
	date := body.TxnDate
	if date == "" {
		date = now[:10]
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	txnID := newID()
	if _, err := tx.Exec(`INSERT INTO inventory_txn(id,item_id,txn_type,quantity,balance_after,reference,note,user_id,txn_date,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, txnID, id, body.TxnType, body.Quantity, newBal, body.Reference, body.Note, sess.userID, date, now); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(`UPDATE inventory_item SET quantity=?, updated_at=? WHERE id=?`, newBal, now, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.db.audit(sess.userID, "inventory_txn", txnID, "create", map[string]any{"type": body.TxnType, "qty": body.Quantity})
	writeJSON(w, 200, map[string]any{"id": txnID, "balance_after": newBal})
}
