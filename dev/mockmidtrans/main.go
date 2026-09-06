// Mock Midtrans untuk pengembangan lokal: menggantikan app.sandbox.midtrans.com
// agar alur pembayaran bisa diuji end-to-end tanpa server key asli.
//
// Endpoint:
//
//	POST /snap/v1/transactions  — menerima request Snap dari goodiebox-server
//	                              (Basic auth: server key), mengembalikan token
//	                              + redirect_url ke halaman simulasi bayar.
//	GET  /snap/pay/{token}      — halaman simulasi (Bayar / Pending).
//	POST /snap/pay/{token}/simulate — memPOST webhook bergambar signature
//	                              sha512 valid ke goodiebox-server, lalu
//	              mengembalikan halaman yang memberi tahu popup pembayaran.
//	GET  /snap.js               — pengganti snap.js resmi: window.snap.pay
//	                              membuka popup halaman simulasi dan meneruskan
//	                              hasil ke callback onSuccess/onPending/onError.
//
// Server key didapat dari Basic auth request Snap, sehingga signature webhook
// selalu konsisten dengan server key yang dipakai goodiebox-server.
package main

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type txn struct {
	OrderID     string
	GrossAmount string // format Midtrans, mis. "49000.00"
	ServerKey   string
}

var (
	mu   sync.Mutex
	txns = map[string]*txn{}
)

var backendURL = strings.TrimRight(envOr("MOCK_BACKEND_URL", "http://localhost:8080"), "/")

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// snapTransactionRequest hanya field yang dibutuhkan mock.
type snapTransactionRequest struct {
	TransactionDetails struct {
		OrderID     string      `json:"order_id"`
		GrossAmount json.Number `json:"gross_amount"`
	} `json:"transaction_details"`
}

// handleCreateTransaction meniru POST /snap/v1/transactions.
func handleCreateTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// SDK Midtrans mengirim Basic auth dengan username = server key.
	user, _, ok := r.BasicAuth()
	if !ok || user == "" {
		writeError(w, http.StatusUnauthorized, "basic auth wajib")
		return
	}
	serverKey := user

	var req snapTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TransactionDetails.OrderID == "" {
		writeError(w, http.StatusBadRequest, "payload tidak valid")
		return
	}

	gross := req.TransactionDetails.GrossAmount.String()
	if !strings.Contains(gross, ".") {
		gross += ".00"
	}
	token := "mocktok-" + randomHex(8)
	mu.Lock()
	txns[token] = &txn{OrderID: req.TransactionDetails.OrderID, GrossAmount: gross, ServerKey: serverKey}
	mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token":        token,
		"redirect_url": fmt.Sprintf("http://%s/snap/pay/%s", r.Host, token),
	})
}

func handleSnapJs(w http.ResponseWriter, r *http.Request) {
	// Origin mock di-bake ke file agar window.snap.pay membuka popup ke
	// mock (mis. :4010), bukan ke origin halaman frontend yang memuatnya.
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = fmt.Fprintf(w, `(function () {
  var MOCK_ORIGIN = %q;
  window.snap = {
    pay: function (token, opts) {
      opts = opts || {}
      var popup = window.open(MOCK_ORIGIN + '/snap/pay/' + token, 'mock-snap', 'width=440,height=680')
      if (popup) popup.focus()
      function onMessage(event) {
        if (event.origin !== MOCK_ORIGIN) return
        var data = event.data || {}
        if (data.mockEvent === 'payment_success' && opts.onSuccess) opts.onSuccess(data)
        else if (data.mockEvent === 'payment_pending' && opts.onPending) opts.onPending(data)
        else if (data.mockEvent === 'payment_error' && opts.onError) opts.onError(data)
        window.removeEventListener('message', onMessage)
      }
      window.addEventListener('message', onMessage)
    }
  }
})()
`, "http://"+r.Host)
}

func handlePayPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/snap/pay/")
	mu.Lock()
	t, ok := txns[token]
	mu.Unlock()
	if !ok {
		http.Error(w, "token tidak dikenal", http.StatusNotFound)
		return
	}
	amountIDR := strings.TrimSuffix(t.GrossAmount, ".00")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="id"><head><meta charset="utf-8"><title>Mock Midtrans — Bayar</title>
<style>
  body { margin:0; font-family: system-ui, sans-serif; background:#f3ebdf; color:#44394a; display:flex; min-height:100vh; align-items:center; justify-content:center; }
  .card { width:min(360px, 92vw); background:#fffaf4; border:1px solid rgba(112,88,132,.25); border-radius:12px; padding:22px; }
  .badge { display:inline-block; margin-bottom:10px; padding:3px 10px; border-radius:999px; background:#8d74a5; color:#fff; font-size:11px; font-weight:700; }
  h1 { margin:0 0 4px; font-size:18px; } p { margin:4px 0; font-size:13px; color:#6e6370; }
  .amount { font-size:26px; font-weight:800; margin:10px 0 16px; }
  button { width:100%%; margin-top:8px; padding:11px; border:0; border-radius:8px; background:#282637; color:#fff; font-size:14px; font-weight:700; cursor:pointer; }
  button.secondary { background:#70568d; }
  small { display:block; margin-top:12px; color:#8a7d8c; font-size:11px; }
</style></head><body>
<div class="card">
  <span class="badge">MOCK SANDBOX</span>
  <h1>Simulasi pembayaran</h1>
  <p>Order <strong>%s</strong></p>
  <div class="amount">Rp %s</div>
  <form method="post" action="/snap/pay/%s/simulate"><input type="hidden" name="status" value="settlement"><button type="submit">Bayar sekarang (settlement)</button></form>
  <form method="post" action="/snap/pay/%s/simulate"><input type="hidden" name="status" value="pending"><button type="submit" class="secondary">Simulasikan pending</button></form>
  <small>Mock Midtrans untuk development — tidak ada uang sungguhan.</small>
</div></body></html>`, t.OrderID, amountIDR, url.PathEscape(token), url.PathEscape(token))
}

// handleSimulate memPOST webhook bergambar signature valid ke goodiebox-server.
func handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/snap/pay/"), "/simulate")
	status := r.FormValue("status")
	mu.Lock()
	t, ok := txns[token]
	mu.Unlock()
	if !ok {
		http.Error(w, "token tidak dikenal", http.StatusNotFound)
		return
	}

	statusCode := map[string]string{"settlement": "200", "pending": "201"}[status]
	if statusCode == "" {
		http.Error(w, "status simulasi tidak dikenal", http.StatusBadRequest)
		return
	}
	transactionID := "mock-txn-" + randomHex(6)
	sum := sha512.Sum512([]byte(t.OrderID + statusCode + t.GrossAmount + t.ServerKey))
	payload := map[string]string{
		"order_id":           t.OrderID,
		"status_code":        statusCode,
		"gross_amount":       t.GrossAmount,
		"transaction_status": status,
		"fraud_status":       "accept",
		"transaction_id":     transactionID,
		"payment_type":       "qris",
		"signature_key":      hex.EncodeToString(sum[:]),
	}

	backendOk := false
	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(backendURL+"/v1/payments/notifications", "application/json", strings.NewReader(string(body)))
	if err == nil {
		backendOk = resp.StatusCode >= 200 && resp.StatusCode < 300
		_ = resp.Body.Close()
	}
	log.Printf("simulasi %s order=%s -> webhook backend ok=%v", status, t.OrderID, backendOk)

	mockEvent := map[string]string{
		"settlement": "payment_success",
		"pending":    "payment_pending",
	}[status]
	if !backendOk {
		mockEvent = "payment_error"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><body style="font-family:system-ui;text-align:center;padding-top:80px;color:#44394a">
<h2>%s</h2><p>Popup akan tertutup otomatis…</p>
<script>
try { window.opener && window.opener.postMessage({ mockEvent: '%s', order_id: %q }, '*'); } catch (e) {}
setTimeout(function () { window.close(); }, 600);
</script></body></html>`,
		map[string]string{
			"payment_success": "✅ Pembayaran berhasil (simulasi)",
			"payment_pending": "⏳ Pembayaran pending (simulasi)",
			"payment_error":   "⚠️ Webhook ke server gagal",
		}[mockEvent], mockEvent, t.OrderID)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/snap/v1/transactions", handleCreateTransaction)
	mux.HandleFunc("/snap.js", handleSnapJs)
	mux.HandleFunc("/snap/pay/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/simulate") {
			handleSimulate(w, r)
			return
		}
		handlePayPage(w, r)
	})

	addr := envOr("MOCK_ADDR", ":4010")
	log.Printf("mock midtrans berjalan di %s (backend: %s)", addr, backendURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
