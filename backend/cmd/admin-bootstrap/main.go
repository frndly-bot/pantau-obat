// Command admin-bootstrap membuat admin pertama pada database.
//
// MASALAH yang dijawab: peran admin tidak pernah bisa diberikan lewat API
// (api-v2 bagian 4 — `role` selalu ditolak sebagai field asing). Akibatnya
// database yang baru dimigrasi tidak punya satu pun admin, seluruh rute
// /admin/* membalas 401, dan uji e2e apidoc berhenti di situ tanpa pernah
// menguji satu endpoint admin pun.
//
// Perintah ini adalah jalur yang disengaja untuk memutus keadaan itu, dan
// HANYA berjalan di luar produksi. Di produksi ia menolak, supaya tidak ada
// jalur belakang yang bisa dipakai menaikkan akun siapa pun jadi admin.
//
// Pemakaian:
//
//	go run ./cmd/admin-bootstrap --email aa@example.com --password 'Rahasia123!' \
//	    --name "Admin Aplikasi" --scope users,catalog,notifications,reports
//
// Tanpa --scope, admin diberi akses penuh (admin_scope NULL).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/frndly-bot/pantau-obat/backend/internal/auth"
	"github.com/frndly-bot/pantau-obat/backend/internal/config"
	"github.com/frndly-bot/pantau-obat/backend/internal/domain"
	"github.com/frndly-bot/pantau-obat/backend/internal/repo/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap admin gagal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		email    = flag.String("email", "", "email admin (wajib)")
		password = flag.String("password", "", "password admin (wajib, minimal 8 karakter)")
		name     = flag.String("name", "", "nama lengkap (wajib bila akun belum ada)")
		scopeRaw = flag.String("scope", "", "cakupan admin, dipisah koma: users,catalog,notifications,reports. Kosong = akses penuh")
	)
	flag.Parse()

	if *email == "" || *password == "" {
		return errors.New("--email dan --password wajib diisi")
	}
	if *name == "" {
		*name = "Admin Aplikasi"
	}

	scope, err := parseScope(*scopeRaw)
	if err != nil {
		return err
	}

	// Kekuatan password diperiksa sebelum menyentuh database. Ini mencegah
	// admin pertama dibuat dengan password lemah hanya karena jalur ini
	// "khusus" dan dianggap tidak perlu diperiksa.
	if err := auth.ValidatePasswordStrength(*password); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Penjagaan produksi. Ini bukan formalitas: jalur ini membuat akun admin
	// tanpa audit siapa yang memicunya, jadi ia hanya layak dipakai pada
	// database yang memang sedang disiapkan.
	if cfg.IsProduction() {
		return errors.New("admin-bootstrap menolak berjalan di APP_ENV=production; " +
			"pakai jalur seed terkontrol")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := postgres.NewDB(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	hash, err := auth.HashPassword(*password)
	if err != nil {
		return err
	}

	res, err := db.BootstrapAdmin(ctx, postgres.BootstrapAdminInput{
		Email:     strings.TrimSpace(*email),
		FullName:  strings.TrimSpace(*name),
		Scope:     scope,
		GrantedBy: nil, // tidak ada aktor; jalur ini memang di luar aplikasi
	}, hash)
	if err != nil {
		return err
	}

	switch {
	case res.Created:
		fmt.Printf("admin dibuat: %s\n", res.UserID)
	case res.Promoted:
		fmt.Printf("akun lama dinaikkan jadi admin: %s\n", res.UserID)
	default:
		fmt.Printf("sudah admin, tidak ada perubahan: %s\n", res.UserID)
	}
	fmt.Printf("email=%s scope=%s\n", *email, scopeString(scope))
	fmt.Println("catatan: login memakai password yang diberikan sekarang.")
	return nil
}

// parseScope mengubah "users,catalog" menjadi daftar AdminScope.
//
// Nilai kosong berarti akses penuh (admin_scope NULL). Nilai yang tidak
// dikenal DITOLAK, bukan diabaikan: cakupan yang salah ketik dan diam-diam
// hilang akan menghasilkan admin yang kehilangan kemampuan tanpa penjelasan.
func parseScope(raw string) ([]domain.AdminScope, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var out []domain.AdminScope
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		s := domain.AdminScope(part)
		if !s.Valid() {
			return nil, fmt.Errorf("cakupan %q tidak dikenal (pilihan: %s)",
				part, scopeString(domain.AllAdminScopes))
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func scopeString(scopes []domain.AdminScope) string {
	if len(scopes) == 0 {
		return "penuh"
	}
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}
