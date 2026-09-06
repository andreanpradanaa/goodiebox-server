package orders

import (
	"errors"
	"time"
)

// ErrNotFound ketika order dengan kode/slug/key tidak ditemukan.
var ErrNotFound = errors.New("order tidak ditemukan")

// ErrNotPaid ketika gift link diakses sebelum pembayaran sukses.
var ErrNotPaid = errors.New("gift belum dibayar")

// Order adalah agregat order gift box.
type Order struct {
	ID          string    `json:"id"`
	OrderCode   string    `json:"order_code"`
	PublicSlug  string    `json:"public_slug"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	SenderName  string    `json:"sender_name"`
	SenderEmail string    `json:"sender_email"`
	SenderPhone string    `json:"sender_phone"`
	PayloadJSON []byte    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	selectOrderCols = `id, order_code, public_slug, status, amount, currency,
		sender_name, sender_email, sender_phone, box_payload, created_at, updated_at`
)

func scanOrder(row interface{ Scan(...any) error }) (*Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.OrderCode, &o.PublicSlug, &o.Status, &o.Amount, &o.Currency,
		&o.SenderName, &o.SenderEmail, &o.SenderPhone, &o.PayloadJSON, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}
