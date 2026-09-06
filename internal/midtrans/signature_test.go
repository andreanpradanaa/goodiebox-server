package midtrans

import (
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	serverKey := "SB-Mid-server-testkey"
	c := New(serverKey, "SB-Mid-client-testkey", false, "")

	valid := func(orderID, statusCode, grossAmount string) string {
		sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
		return hex.EncodeToString(sum[:])
	}

	tests := []struct {
		name string
		n    Notification
		want bool
	}{
		{
			name: "signature valid",
			n: Notification{
				OrderID: "GBX-260906-ABCDE", StatusCode: "200",
				GrossAmount: "49000.00", SignatureKey: valid("GBX-260906-ABCDE", "200", "49000.00"),
			},
			want: true,
		},
		{
			name: "gross_amount berbeda maka signature tidak cocok",
			n: Notification{
				OrderID: "GBX-260906-ABCDE", StatusCode: "200",
				GrossAmount: "99000.00", SignatureKey: valid("GBX-260906-ABCDE", "200", "49000.00"),
			},
			want: false,
		},
		{
			name: "signature kosong",
			n:    Notification{OrderID: "GBX-260906-ABCDE", StatusCode: "200", GrossAmount: "49000.00"},
			want: false,
		},
		{
			name: "signature acak",
			n: Notification{
				OrderID: "GBX-260906-ABCDE", StatusCode: "200", GrossAmount: "49000.00",
				SignatureKey: "deadbeef",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.VerifySignature(tt.n); got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}
