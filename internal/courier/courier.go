// courier-service: Features #7, #8 (courier side) and #10 (admin assignment).
package courier

import (
	"database/sql"
	"log"
	"net/http"

	"package-tracking/internal/common"
)

type app struct{ db *sql.DB }

// previous status required for each courier-driven status update
var prev = map[string]string{"picked_up": "accepted", "in_transit": "picked_up", "delivered": "in_transit"}

func serverError(w http.ResponseWriter, err error) {
	log.Println("error:", err)
	common.Error(w, http.StatusInternalServerError, "internal server error")
}

// GET /api/couriers/{id}/orders  (Feature #7)
func (a *app) assigned(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid courier id")
		return
	}
	c := common.CurrentUser(r)
	if c.Role != "admin" && c.UserID != id {
		common.Error(w, http.StatusForbidden, "you can only view your own assigned orders")
		return
	}
	orders, err := common.QueryOrders(a.db, "WHERE o.courier_id=? ORDER BY o.delivery_time", id)
	if err != nil {
		serverError(w, err)
		return
	}
	common.JSON(w, http.StatusOK, orders)
}

// transition atomically moves an order the calling courier owns from one status to another.
func (a *app) transition(w http.ResponseWriter, r *http.Request, orderID int64, from, to, note string, release bool) {
	c := common.CurrentUser(r)
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	q := `UPDATE orders SET status=? WHERE id=? AND courier_id=? AND status=?`
	if release {
		q = `UPDATE orders SET status=?, courier_id=NULL WHERE id=? AND courier_id=? AND status=?`
	}
	res, err := tx.Exec(q, to, orderID, c.UserID, from)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.Error(w, http.StatusConflict, "order not found for this courier, or it is not in '"+from+"' status")
		return
	}
	if err := common.AddHistory(tx, orderID, to, c.UserID, note); err != nil {
		serverError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	o, _ := common.GetOrder(a.db, orderID)
	common.JSON(w, http.StatusOK, o)
}

// POST /api/courier/orders/{id}/accept  (Feature #7)
func (a *app) accept(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	a.transition(w, r, id, "assigned", "accepted", "accepted by courier", false)
}

// POST /api/courier/orders/{id}/decline  (Feature #7) - order goes back to the pending pool
func (a *app) decline(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	a.transition(w, r, id, "assigned", "pending", "declined by courier", true)
}

// PATCH /api/courier/orders/{id}/status  (Feature #8)
func (a *app) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	from, ok := prev[in.Status]
	if !ok {
		common.Error(w, http.StatusBadRequest, "status must be one of: picked_up, in_transit, delivered")
		return
	}
	a.transition(w, r, id, from, in.Status, "updated by courier", false)
}

// GET /api/admin/assignments  (Feature #10) - every order with its courier (if any)
func (a *app) assignments(w http.ResponseWriter, r *http.Request) {
	orders, err := common.QueryOrders(a.db, "ORDER BY (o.courier_id IS NULL) DESC, o.created_at DESC")
	if err != nil {
		serverError(w, err)
		return
	}
	common.JSON(w, http.StatusOK, orders)
}

// PUT /api/admin/orders/{id}/assign  (Feature #10) - assign or reassign
func (a *app) assign(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	var in struct {
		CourierID int64 `json:"courier_id"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	var n int
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=? AND role='courier'`, in.CourierID).Scan(&n)
	if n == 0 {
		common.Error(w, http.StatusBadRequest, "courier_id is not a valid courier")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE orders SET courier_id=?, status='assigned' WHERE id=? AND status IN ('pending','assigned','accepted')`, in.CourierID, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		common.Error(w, http.StatusConflict, "order not found or can no longer be (re)assigned")
		return
	}
	_ = common.AddHistory(tx, id, "assigned", common.CurrentUser(r).UserID, "assigned by admin")
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	o, _ := common.GetOrder(a.db, id)
	common.JSON(w, http.StatusOK, o)
}

func Register(mux *http.ServeMux, db *sql.DB) {
	a := &app{db: db}
	mux.HandleFunc("GET /api/couriers/{id}/orders", common.Protect(a.assigned, "courier", "admin"))
	mux.HandleFunc("POST /api/courier/orders/{id}/accept", common.Protect(a.accept, "courier"))
	mux.HandleFunc("POST /api/courier/orders/{id}/decline", common.Protect(a.decline, "courier"))
	mux.HandleFunc("PATCH /api/courier/orders/{id}/status", common.Protect(a.updateStatus, "courier"))
	mux.HandleFunc("GET /api/admin/assignments", common.Protect(a.assignments, "admin"))
	mux.HandleFunc("PUT /api/admin/orders/{id}/assign", common.Protect(a.assign, "admin"))
}
