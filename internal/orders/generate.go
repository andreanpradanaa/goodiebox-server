package orders

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// Charset tanpa karakter mudah tertukar (0/O, 1/I/L) karena
// order code dan slug bisa diketik ulang manual.
const (
	codeChars = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	slugChars = "abcdefghjkmnpqrstuvwxyz23456789"
)

// NewOrderCode menghasilkan kode order unik sekaligus idempotency key
// di sisi Midtrans, format GBX-YYMMDD-XXXXXX.
func NewOrderCode(t time.Time) string {
	return fmt.Sprintf("GBX-%s-%s", t.Format("060102"), randomChars(codeChars, 6))
}

// NewPublicSlug menghasilkan slug link kejutan publik.
func NewPublicSlug() string {
	return randomChars(slugChars, 10)
}

func randomChars(charset string, n int) string {
	out := make([]byte, n)
	max := big.NewInt(int64(len(charset)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand gagal hanya saat OS kehabisan entropi;
			// lebih baik panik daripada membuat kode yang mudah ditebak.
			panic(fmt.Sprintf("crypto/rand gagal: %v", err))
		}
		out[i] = charset[idx.Int64()]
	}
	return string(out)
}
