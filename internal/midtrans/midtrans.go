// Package midtrans membungkus integrasi Midtrans Snap: membuat transaksi,
// verifikasi signature webhook, pemetaan status, dan parsing gross_amount.
package midtrans

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/midtrans/midtrans-go"
	"github.com/midtrans/midtrans-go/snap"
)

// Client adalah wrapper tipis di atas SDK resmi Midtrans.
type Client struct {
	snap       snap.Client
	serverKey  string
	clientKey  string
	production bool
}

func New(serverKey, clientKey string, production bool) *Client {
	var sc snap.Client
	env := midtrans.Sandbox
	if production {
		env = midtrans.Production
	}
	sc.New(serverKey, env)
	return &Client{snap: sc, serverKey: serverKey, clientKey: clientKey, production: production}
}

func (c *Client) ClientKey() string  { return c.clientKey }
func (c *Client) IsProduction() bool { return c.production }
func (c *Client) ServerKey() string  { return c.serverKey }

// Customer adalah data pembayar yang diteruskan ke Midtrans.
type Customer struct {
	Name  string
	Email string
	Phone string
}

// CreateSnapTransaction meminta Snap token untuk satu order.
// orderCode dipakai sebagai order_id di sisi Midtrans sehingga menjadi
// idempotency key: Midtrans tidak memproses order yang sama dua kali.
func (c *Client) CreateSnapTransaction(ctx context.Context, orderCode string, amountIDR int64, cust Customer) (token, redirectURL string, err error) {
	req := &snap.Request{
		TransactionDetails: midtrans.TransactionDetails{
			OrderID:  orderCode,
			GrossAmt: amountIDR,
		},
		Items: &[]midtrans.ItemDetails{{
			ID:    "goodiebox-1",
			Price: amountIDR,
			Qty:   1,
			Name:  "GoodieBox Digital Gift Box",
		}},
		CustomerDetail: &midtrans.CustomerDetails{
			FName: cust.Name,
			Email: cust.Email,
			Phone: cust.Phone,
		},
		Expiry: &snap.ExpiryDetails{Unit: "hour", Duration: 24},
	}

	resp, mErr := c.snap.CreateTransaction(req)
	if mErr != nil {
		return "", "", fmt.Errorf("midtrans snap gagal (order %s): %s", orderCode, mErr.Message)
	}
	return resp.Token, resp.RedirectURL, nil
}

// Notification adalah subset field HTTP notification Midtrans yang relevan.
// Payload mentah lengkap tetap disimpan untuk audit.
type Notification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
	PaymentType       string `json:"payment_type"`
	SignatureKey      string `json:"signature_key"`
	StatusMessage     string `json:"status_message"`
}

// VerifySignature memeriksa signature_key webhook:
// sha512(order_id + status_code + gross_amount + server_key).
// Notifikasi tanpa signature valid tidak boleh mengubah status pembayaran.
func (c *Client) VerifySignature(n Notification) bool {
	if n.SignatureKey == "" {
		return false
	}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + c.serverKey))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(n.SignatureKey)) == 1
}

// State status pembayaran internal (kolom payments.status).
type PaymentState string

const (
	PaymentPending   PaymentState = "pending"
	PaymentSuccess   PaymentState = "success"
	PaymentFailed    PaymentState = "failed"
	PaymentExpired   PaymentState = "expired"
	PaymentCancelled PaymentState = "cancelled"
	PaymentRefunded  PaymentState = "refunded"
)

// State status order internal (kolom orders.status).
type OrderState string

const (
	OrderPending   OrderState = "pending"
	OrderPaid      OrderState = "paid"
	OrderExpired   OrderState = "expired"
	OrderCancelled OrderState = "cancelled"
)

// MapStatus menerjemahkan kombinasi transaction_status + fraud_status Midtrans
// menjadi status internal. Dokumentasi:
// https://docs.midtrans.com/docs/transaction-statuses-and-actions
func MapStatus(transactionStatus, fraudStatus string) (PaymentState, OrderState) {
	switch transactionStatus {
	case "capture":
		if fraudStatus == "challenge" {
			return PaymentPending, OrderPending // 3DS/review: tunggu notifikasi lanjutan
		}
		return PaymentSuccess, OrderPaid
	case "settlement":
		return PaymentSuccess, OrderPaid
	case "pending":
		return PaymentPending, OrderPending
	case "deny":
		return PaymentFailed, OrderPending // pembayar boleh coba metode lain
	case "cancel":
		return PaymentCancelled, OrderCancelled
	case "expire":
		return PaymentExpired, OrderExpired
	case "refund", "partial_refund":
		return PaymentRefunded, OrderPaid // link tetap aktif, dana kembali
	default:
		return PaymentPending, OrderPending
	}
}

// ParseGrossAmountIDR mengubah gross_amount Midtrans ("49000" atau "49000.00")
// menjadi IDR bulat. Amount tidak pernah disimpan sebagai float.
func ParseGrossAmountIDR(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("gross_amount kosong")
	}
	intPart, fracPart := s, ""
	if i := strings.Index(s, "."); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
	}
	if intPart == "" || fracPart != "" && fracPart != "0" && fracPart != "00" {
		return 0, fmt.Errorf("gross_amount %q bukan IDR bulat yang valid", s)
	}
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("gross_amount %q tidak valid: %w", s, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("gross_amount %q harus positif", s)
	}
	return n, nil
}
