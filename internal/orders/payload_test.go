package orders

import (
	"strings"
	"testing"
	"time"
)

func TestNewOrderCodeFormat(t *testing.T) {
	code := NewOrderCode(time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(code, "GBX-260906-") {
		t.Errorf("format order code salah: %q", code)
	}
	suffix := strings.TrimPrefix(code, "GBX-260906-")
	if len(suffix) != 6 {
		t.Errorf("panjang suffix harus 6, got %q", suffix)
	}
}

func TestNewOrderCodeUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		code := NewOrderCode(time.Now().UTC())
		if seen[code] {
			t.Fatalf("order code duplikat: %q", code)
		}
		seen[code] = true
	}
}

func TestNewPublicSlug(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		slug := NewPublicSlug()
		if len(slug) != 10 {
			t.Fatalf("panjang slug harus 10, got %q", slug)
		}
		if seen[slug] {
			t.Fatalf("slug duplikat: %q", slug)
		}
		seen[slug] = true
	}
}

func TestCreateRequestValidate(t *testing.T) {
	valid := CreateRequest{
		RecipientName: "Adik",
		SenderName:    "Kakak",
		Mood:          "warm",
		BoxColor:      "#ff0000",
		Items:         []byte(`[{"id":"letter-1","type":"letter","message":"hai"}]`),
		Customer:      Customer{Email: "kakak@example.com"},
	}

	tests := []struct {
		name    string
		mutate  func(*CreateRequest)
		wantErr bool
	}{
		{"request valid", func(*CreateRequest) {}, false},
		{"recipient kosong", func(r *CreateRequest) { r.RecipientName = "" }, true},
		{"mood tidak dikenal", func(r *CreateRequest) { r.Mood = "angry" }, true},
		{"boxColor kosong", func(r *CreateRequest) { r.BoxColor = "" }, true},
		{"email tidak valid", func(r *CreateRequest) { r.Customer.Email = "bukan-email" }, true},
		{"items bukan array", func(r *CreateRequest) { r.Items = []byte(`"letter"`) }, true},
		{"tipe item tidak dikenal", func(r *CreateRequest) { r.Items = []byte(`[{"type":"uang"}]`) }, true},
		{"lebih dari 6 item", func(r *CreateRequest) {
			r.Items = []byte(`[{"type":"letter"},{"type":"photo"},{"type":"music"},{"type":"voucher"},{"type":"audio"},{"type":"video"},{"type":"letter"}]`)
		}, true},
		{"tepat 6 item", func(r *CreateRequest) {
			r.Items = []byte(`[{"type":"letter"},{"type":"photo"},{"type":"music"},{"type":"voucher"},{"type":"audio"},{"type":"video"}]`)
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.mutate(&req)
			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoxPayloadJSONStripsCustomer(t *testing.T) {
	req := CreateRequest{
		RecipientName: "Adik",
		SenderName:    "Kakak",
		Mood:          "warm",
		BoxColor:      "#ff0000",
		BoxTheme:      "polkadot",
		Items:         []byte(`[{"id":"letter-1","type":"letter","message":"rahasia"}]`),
		Customer:      Customer{Name: "Kakak", Email: "kakak@example.com", Phone: "0812"},
	}

	raw, err := req.BoxPayloadJSON()
	if err != nil {
		t.Fatalf("BoxPayloadJSON() error: %v", err)
	}

	// Data customer tidak boleh ikut: payload ini dikirim ke halaman penerima.
	for _, leaked := range []string{"kakak@example.com", "0812", "customer"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("payload membocorkan %q: %s", leaked, raw)
		}
	}
	// Field builder tetap ada, termasuk isi item yang tidak divalidasi detailnya.
	for _, want := range []string{"Adik", "Kakak", "warm", "polkadot", "rahasia"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("payload kehilangan %q: %s", want, raw)
		}
	}
}

func TestCustomerForPaymentFallsBackToSenderName(t *testing.T) {
	req := CreateRequest{SenderName: "Kakak", Customer: Customer{Email: "k@example.com"}}
	cust := req.CustomerForPayment()
	if cust.Name != "Kakak" {
		t.Errorf("CustomerForPayment().Name = %q, want %q", cust.Name, "Kakak")
	}
}
