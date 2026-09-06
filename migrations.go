// Package migrations menyematkan file SQL di folder migrations/ agar
// bisa dijalankan server saat startup tanpa tooling tambahan.
package migrations

import "embed"

//go:embed migrations/*.sql
var FS embed.FS
