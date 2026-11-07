package common

import (
	"database/sql"
	"regexp"
	"time"
)

var ValidStatuses = map[string]bool{
	"pending": true, "assigned": true, "accepted": true,
	"picked_up": true, "in_transit": true, "delivered": true, "cancelled": true,
}

// PhoneRe is the phone format accepted for users and for an order's customer.
var PhoneRe = regexp.MustCompile(`^\+?[0-9]{8,15}$`)

type Courier struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Seller is the customer account that created the order; the courier calls them to arrange pickup.
type Seller struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type Order struct {
	ID              int64     `json:"id"`
	UserID          int64     `json:"user_id"`
	Seller          *Seller   `json:"seller,omitempty"`
	PickupLocation  string    `json:"pickup_location"`
	DropoffLocation string    `json:"dropoff_location"`
	CustomerName    string    `json:"customer_name"`
	CustomerPhone   string    `json:"customer_phone"`
	PackageDetails  string    `json:"package_details"`
	DeliveryTime    time.Time `json:"delivery_time"`
	DeliveryFee     float64   `json:"delivery_fee"`
	CollectAmount   float64   `json:"collect_amount"`
	Status          string    `json:"status"`
	CourierID       *int64    `json:"courier_id"`
	Courier         *Courier  `json:"courier,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const orderSelect = `SELECT o.id,o.user_id,s.name,s.phone,o.pickup_location,o.dropoff_location,
 o.customer_name,o.customer_phone,o.package_details,o.delivery_time,o.delivery_fee,o.collect_amount,
 o.status,o.courier_id,c.name,c.phone,o.created_at,o.updated_at
 FROM orders o LEFT JOIN users c ON c.id=o.courier_id LEFT JOIN users s ON s.id=o.user_id `

func QueryOrders(db *sql.DB, clause string, args ...any) ([]Order, error) {
	rows, err := db.Query(orderSelect+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Order{}
	for rows.Next() {
		var o Order
		var cid sql.NullInt64
		var cn, cp, sn, sp sql.NullString
		if err := rows.Scan(&o.ID, &o.UserID, &sn, &sp, &o.PickupLocation, &o.DropoffLocation,
			&o.CustomerName, &o.CustomerPhone, &o.PackageDetails, &o.DeliveryTime, &o.DeliveryFee, &o.CollectAmount,
			&o.Status, &cid, &cn, &cp, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		if sn.Valid {
			o.Seller = &Seller{Name: sn.String, Phone: sp.String}
		}
		if cid.Valid {
			id := cid.Int64
			o.CourierID = &id
			o.Courier = &Courier{ID: id, Name: cn.String, Phone: cp.String}
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func GetOrder(db *sql.DB, id int64) (*Order, error) {
	orders, err := QueryOrders(db, "WHERE o.id=?", id)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, sql.ErrNoRows
	}
	return &orders[0], nil
}

type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func AddHistory(e Execer, orderID int64, status string, changedBy int64, note string) error {
	_, err := e.Exec(`INSERT INTO order_status_history (order_id,status,changed_by,note) VALUES (?,?,?,?)`,
		orderID, status, changedBy, note)
	return err
}
