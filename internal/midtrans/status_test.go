package midtrans

import "testing"

func TestMapStatus(t *testing.T) {
	tests := []struct {
		txnStatus   string
		fraudStatus string
		wantPayment PaymentState
		wantOrder   OrderState
	}{
		{"capture", "accept", PaymentSuccess, OrderPaid},
		{"capture", "challenge", PaymentPending, OrderPending},
		{"settlement", "", PaymentSuccess, OrderPaid},
		{"pending", "", PaymentPending, OrderPending},
		{"deny", "", PaymentFailed, OrderPending},
		{"cancel", "", PaymentCancelled, OrderCancelled},
		{"expire", "", PaymentExpired, OrderExpired},
		{"refund", "", PaymentRefunded, OrderPaid},
		{"partial_refund", "", PaymentRefunded, OrderPaid},
		{"status-tidak-dikenal", "", PaymentPending, OrderPending},
	}

	for _, tt := range tests {
		t.Run(tt.txnStatus+"/"+tt.fraudStatus, func(t *testing.T) {
			gotPay, gotOrder := MapStatus(tt.txnStatus, tt.fraudStatus)
			if gotPay != tt.wantPayment || gotOrder != tt.wantOrder {
				t.Errorf("MapStatus(%q, %q) = (%s, %s), want (%s, %s)",
					tt.txnStatus, tt.fraudStatus, gotPay, gotOrder, tt.wantPayment, tt.wantOrder)
			}
		})
	}
}

func TestParseGrossAmountIDR(t *testing.T) {
	tests := []struct {
		in    string
		want  int64
		isErr bool
	}{
		{"49000", 49000, false},
		{"49000.00", 49000, false},
		{"49000.0", 49000, false},
		{" 49000.00 ", 49000, false},
		{"49000.50", 0, true}, // IDR tidak punya pecahan di sistem ini
		{"abc", 0, true},
		{"", 0, true},
		{"0", 0, true},
		{"-100", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseGrossAmountIDR(tt.in)
			if tt.isErr {
				if err == nil {
					t.Fatalf("ParseGrossAmountIDR(%q) tidak error, got %d", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGrossAmountIDR(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseGrossAmountIDR(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
