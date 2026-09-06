package orders

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Item type mengikuti katalog di frontend goodiebox-v2 (src/goodiebox/itemCatalog.ts).
var allowedItemTypes = map[string]bool{
	"letter": true, "photo": true, "music": true,
	"voucher": true, "audio": true, "video": true,
}

var allowedMoods = map[string]bool{
	"warm": true, "romantic": true, "birthday": true,
	"friendship": true, "thanks": true, "encouragement": true,
}

var allowedThemes = map[string]bool{
	"": true, "plain": true, "polkadot": true, "stripes": true, "botanical": true,
}

const maxItems = 6

// CreateRequest adalah body POST /v1/orders: builder state frontend
// (GoodieBoxBuilderState) + kontak pengirim.
type CreateRequest struct {
	RecipientName string          `json:"recipientName"`
	SenderName    string          `json:"senderName"`
	Mood          string          `json:"mood"`
	InnerNote     string          `json:"innerNote"`
	BoxColor      string          `json:"boxColor"`
	BoxTheme      string          `json:"boxTheme"`
	Items         json.RawMessage `json:"items"`
	Customer      Customer        `json:"customer"`
}

// Customer adalah data pembayar (pengirim) untuk Midtrans.
type Customer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func (r *CreateRequest) Validate() error {
	if strings.TrimSpace(r.RecipientName) == "" {
		return &ValidationError{"recipientName wajib diisi"}
	}
	if strings.TrimSpace(r.SenderName) == "" {
		return &ValidationError{"senderName wajib diisi"}
	}
	if !allowedMoods[r.Mood] {
		return &ValidationError{fmt.Sprintf("mood %q tidak dikenal", r.Mood)}
	}
	if !allowedThemes[r.BoxTheme] {
		return &ValidationError{fmt.Sprintf("boxTheme %q tidak dikenal", r.BoxTheme)}
	}
	if strings.TrimSpace(r.BoxColor) == "" {
		return &ValidationError{"boxColor wajib diisi"}
	}
	if strings.TrimSpace(r.Customer.Email) == "" || !strings.Contains(r.Customer.Email, "@") {
		return &ValidationError{"customer.email wajib diisi dengan email valid"}
	}

	var items []struct {
		Type string `json:"type"`
	}
	if len(r.Items) == 0 {
		return &ValidationError{"items wajib ada (boleh array kosong)"}
	}
	if err := json.Unmarshal(r.Items, &items); err != nil {
		return &ValidationError{"items harus berupa array"}
	}
	if len(items) > maxItems {
		return &ValidationError{fmt.Sprintf("maksimal %d item per box", maxItems)}
	}
	for _, it := range items {
		if !allowedItemTypes[it.Type] {
			return &ValidationError{fmt.Sprintf("tipe item %q tidak dikenal", it.Type)}
		}
	}
	return nil
}

// BoxPayloadJSON menyerialisasi builder state tanpa data customer,
// karena payload inilah yang nanti dikirim ke halaman penerima.
func (r *CreateRequest) BoxPayloadJSON() ([]byte, error) {
	return json.Marshal(struct {
		RecipientName string          `json:"recipientName"`
		SenderName    string          `json:"senderName"`
		Mood          string          `json:"mood"`
		InnerNote     string          `json:"innerNote"`
		BoxColor      string          `json:"boxColor"`
		BoxTheme      string          `json:"boxTheme,omitempty"`
		Items         json.RawMessage `json:"items"`
	}{
		RecipientName: r.RecipientName,
		SenderName:    r.SenderName,
		Mood:          r.Mood,
		InnerNote:     r.InnerNote,
		BoxColor:      r.BoxColor,
		BoxTheme:      r.BoxTheme,
		Items:         r.Items,
	})
}

// CustomerForPayment mengembalikan data pembayar; nama kosong
// diisi dengan senderName agar detail customer Midtrans tetap lengkap.
func (r *CreateRequest) CustomerForPayment() Customer {
	name := strings.TrimSpace(r.Customer.Name)
	if name == "" {
		name = r.SenderName
	}
	return Customer{Name: name, Email: r.Customer.Email, Phone: r.Customer.Phone}
}
