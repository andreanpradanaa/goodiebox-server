package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect membuat connection pool pgx yang dipakai seluruh service.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat pool postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("gagal terhubung ke postgres: %w", err)
	}
	return pool, nil
}

// Migrate menjalankan semua migrasi SQL yang belum diterapkan.
// fsys harus berisi file migrations/*.sql (embed.FS dari package migrations).
// Dipanggil sebelum server menerima trafik; migrasi bersifat idempotent.
func Migrate(databaseURL string, fsys embed.FS) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("gagal membuka koneksi migrasi: %w", err)
	}
	defer sqlDB.Close()

	src, err := iofs.New(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("gagal membaca file migrasi: %w", err)
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("gagal inisialisasi driver migrasi: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("gagal inisialisasi migrasi: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("gagal menjalankan migrasi: %w", err)
	}
	return nil
}
