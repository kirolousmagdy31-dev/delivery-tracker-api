// order-service: Features #4, #5, #6 (backend) and #9 (admin manage orders).
package order

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"package-tracking/internal/common"
)

type app struct{ db *sql.DB }

type historyItem struct {
	Status    string    `json:"status"`
	ChangedBy int64     `json:"changed_by"`
	Note      string    `json:"note"`
	At        time.Time `json:"at"`
}

func serverError(w http.ResponseWriter, err error) {
	log.Println("error:", err)
	common.Error(w, http.StatusInternalServerError, "internal server error")
}

// POST /api/orders  (Feature #4)
func (a *app) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PickupLocation  string  `json:"pickup_location"`
		DropoffLocation string  `json:"dropoff_location"`
		PackageDetails  string  `json:"package_details"`
		DeliveryTime    string  `json:"delivery_time"`
		DeliveryFee     float64 `json:"delivery_fee"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.PickupLocation = strings.TrimSpace(in.PickupLocation)
	in.DropoffLocation = strings.TrimSpace(in.DropoffLocation)
	in.PackageDetails = strings.TrimSpace(in.PackageDetails)
	when, err := common.ParseTime(in.DeliveryTime)
	switch {
	case in.PickupLocation == "" || in.DropoffLocation == "" || in.PackageDetails == "":
		common.Error(w, http.StatusBadRequest, "pickup_location, dropoff_location and package_details are required")
		return
	case err != nil:
		common.Error(w, http.StatusBadRequest, "delivery_time must be like 2026-11-10T14:00 or RFC3339")
		return
	case when.Before(time.Now().Add(-24 * time.Hour)):
		common.Error(w, http.StatusBadRequest, "delivery_time cannot be in the past")
		return
	case in.DeliveryFee < 0:
		common.Error(w, http.StatusBadRequest, "delivery_fee cannot be negative")
		return
	}
	uid := common.CurrentUser(r).UserID
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO orders (user_id,pickup_location,dropoff_location,package_details,delivery_time,delivery_fee)
		VALUES (?,?,?,?,?,?)`, uid, in.PickupLocation, in.DropoffLocation, in.PackageDetails, when, in.DeliveryFee)
	if err != nil {
		serverError(w, err)
		return
	}
	id, _ := res.LastInsertId()
	if err := common.AddHistory(tx, id, "pending", uid, "order created"); err != nil {
		serverError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	o, _ := common.GetOrder(a.db, id)
	common.JSON(w, http.StatusCreated, o)
}

// GET /api/orders/my  (Feature #5)
func (a *app) mine(w http.ResponseWriter, r *http.Request) {
	orders, err := common.QueryOrders(a.db, "WHERE o.user_id=? ORDER BY o.created_at DESC", common.CurrentUser(r).UserID)
	if err != nil {
		serverError(w, err)
		return
	}
	common.JSON(w, http.StatusOK, orders)
}

// GET /api/orders/{id}  (Feature #6)
func (a *app) details(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	o, err := common.GetOrder(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		common.Error(w, http.StatusNotFound, "order not found")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	c := common.CurrentUser(r)
	allowed := c.Role == "admin" || (c.Role == "user" && o.UserID == c.UserID) ||
		(c.Role == "courier" && o.CourierID != nil && *o.CourierID == c.UserID)
	if !allowed {
		common.Error(w, http.StatusForbidden, "you cannot view this order")
		return
	}
	hist := []historyItem{}
	rows, err := a.db.Query(`SELECT status,changed_by,note,created_at FROM order_status_history WHERE order_id=? ORDER BY id`, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var h historyItem
			if rows.Scan(&h.Status, &h.ChangedBy, &h.Note, &h.At) == nil {
				hist = append(hist, h)
			}
		}
	}
	common.JSON(w, http.StatusOK, struct {
		*common.Order
		History []historyItem `json:"history"`
	}{o, hist})
}

// PATCH /api/orders/{id}/cancel  (Feature #6 - cancel pending order)
func (a *app) cancel(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	uid := common.CurrentUser(r).UserID
	o, err := common.GetOrder(a.db, id)
	if err != nil || o.UserID != uid {
		common.Error(w, http.StatusNotFound, "order not found")
		return
	}
	if o.Status != "pending" {
		common.Error(w, http.StatusConflict, "only pending orders can be cancelled (current status: "+o.Status+")")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE orders SET status='cancelled' WHERE id=? AND user_id=? AND status='pending'`, id, uid)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.Error(w, http.StatusConflict, "order is no longer pending")
		return
	}
	_ = common.AddHistory(tx, id, "cancelled", uid, "cancelled by customer")
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	o, _ = common.GetOrder(a.db, id)
	common.JSON(w, http.StatusOK, o)
}

// GET /api/admin/orders  (Feature #9)
func (a *app) adminList(w http.ResponseWriter, r *http.Request) {
	orders, err := common.QueryOrders(a.db, "ORDER BY o.created_at DESC")
	if err != nil {
		serverError(w, err)
		return
	}
	common.JSON(w, http.StatusOK, orders)
}

// PUT /api/admin/orders/{id}  (Feature #9 - update order / status)
func (a *app) adminUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	var in struct {
		PickupLocation  *string  `json:"pickup_location"`
		DropoffLocation *string  `json:"dropoff_location"`
		PackageDetails  *string  `json:"package_details"`
		DeliveryTime    *string  `json:"delivery_time"`
		DeliveryFee     *float64 `json:"delivery_fee"`
		Status          *string  `json:"status"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	o, err := common.GetOrder(a.db, id)
	if errors.Is(err, sql.ErrNoRows) {
		common.Error(w, http.StatusNotFound, "order not found")
		return
	} else if err != nil {
		serverError(w, err)
		return
	}
	oldStatus := o.Status
	if in.PickupLocation != nil {
		o.PickupLocation = strings.TrimSpace(*in.PickupLocation)
	}
	if in.DropoffLocation != nil {
		o.DropoffLocation = strings.TrimSpace(*in.DropoffLocation)
	}
	if in.PackageDetails != nil {
		o.PackageDetails = strings.TrimSpace(*in.PackageDetails)
	}
	if in.DeliveryFee != nil {
		if *in.DeliveryFee < 0 {
			common.Error(w, http.StatusBadRequest, "delivery_fee cannot be negative")
			return
		}
		o.DeliveryFee = *in.DeliveryFee
	}
	if in.DeliveryTime != nil {
		t, err := common.ParseTime(*in.DeliveryTime)
		if err != nil {
			common.Error(w, http.StatusBadRequest, "invalid delivery_time")
			return
		}
		o.DeliveryTime = t
	}
	if in.Status != nil {
		if !common.ValidStatuses[*in.Status] {
			common.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		o.Status = *in.Status
	}
	if o.PickupLocation == "" || o.DropoffLocation == "" || o.PackageDetails == "" {
		common.Error(w, http.StatusBadRequest, "locations and package details cannot be empty")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE orders SET pickup_location=?,dropoff_location=?,package_details=?,delivery_time=?,delivery_fee=?,status=? WHERE id=?`,
		o.PickupLocation, o.DropoffLocation, o.PackageDetails, o.DeliveryTime, o.DeliveryFee, o.Status, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if o.Status != oldStatus {
		_ = common.AddHistory(tx, id, o.Status, common.CurrentUser(r).UserID, "updated by admin")
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	o, _ = common.GetOrder(a.db, id)
	common.JSON(w, http.StatusOK, o)
}

// DELETE /api/admin/orders/{id}  (Feature #9)
func (a *app) adminDelete(w http.ResponseWriter, r *http.Request) {
	id, err := common.PathID(r)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "invalid order id")
		return
	}
	res, err := a.db.Exec(`DELETE FROM orders WHERE id=?`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.Error(w, http.StatusNotFound, "order not found")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "order deleted"})
}

func Register(mux *http.ServeMux, db *sql.DB) {
	a := &app{db: db}
	mux.HandleFunc("POST /api/orders", common.Protect(a.create, "user"))
	mux.HandleFunc("GET /api/orders/my", common.Protect(a.mine, "user"))
	mux.HandleFunc("GET /api/orders/{id}", common.Protect(a.details))
	mux.HandleFunc("PATCH /api/orders/{id}/cancel", common.Protect(a.cancel, "user"))
	mux.HandleFunc("GET /api/admin/orders", common.Protect(a.adminList, "admin"))
	mux.HandleFunc("PUT /api/admin/orders/{id}", common.Protect(a.adminUpdate, "admin"))
	mux.HandleFunc("DELETE /api/admin/orders/{id}", common.Protect(a.adminDelete, "admin"))
}
