package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config menampung pengaturan object storage.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Enabled melaporkan apakah penyimpanan objek dikonfigurasi.
//
// Tanpa ini, fitur foto resep harus bersikap jelas: menolak unggahan dengan
// pesan "belum dikonfigurasi", BUKAN menerimanya lalu menghilangkannya
// diam-diam. Pengguna yang memotret resep lalu diberi tahu "berhasil" padahal
// berkasnya tidak tersimpan akan kehilangan dokumen yang ia kira aman.
func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.Bucket != ""
}

// Photos menyimpan foto resep di object storage S3-compatible (pgsty/silo).
type Photos struct {
	cfg    Config
	client *minio.Client
	logger *slog.Logger
}

// ErrPhotoTooLarge dikembalikan bila berkas melewati batas ukuran.
var ErrPhotoTooLarge = errors.New("berkas terlalu besar")

// ErrPhotoTypeTidakDidukung dikembalikan untuk jenis berkas di luar daftar.
var ErrPhotoTypeTidakDidukung = errors.New("jenis berkas tidak didukung")

// MaxPhotoBytes adalah batas ukuran satu foto resep (8 MiB).
//
// Dipilih setelah mempertimbangkan kamera ponsel: foto resolusi penuh dari
// kamera modern bisa 3–6 MiB, dan pengguna memotret resep apa adanya —
// bukan hasil yang sudah dikompres. Batas yang terlalu kecil akan menolak
// foto yang sah, dan itu membuat fiturnya tampak rusak.
const MaxPhotoBytes = 8 << 20

// NewPhotos menyiapkan klien penyimpanan.
//
// Bila konfigurasinya kosong, mengembalikan (nil, nil) — pemanggil memeriksa
// nil dan bersikap seolah fitur belum tersedia. Ini disengaja: aplikasi harus
// tetap jalan walau object storage belum dipasang.
func NewPhotos(ctx context.Context, cfg Config, logger *slog.Logger) (*Photos, error) {
	if !cfg.Enabled() {
		logger.InfoContext(ctx, "penyimpanan foto resep belum dikonfigurasi")
		return nil, nil
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("menyiapkan klien penyimpanan: %w", err)
	}

	p := &Photos{cfg: cfg, client: client, logger: logger}

	// Bucket disiapkan saat start. Kalau gagal, JANGAN gagalkan startup
	// seluruh aplikasi: pencatatan obat tetap harus jalan. Cukup dicatat.
	if err := p.ensureBucket(ctx); err != nil {
		logger.WarnContext(ctx, "bucket foto resep belum siap; unggahan akan gagal sampai diperbaiki",
			slog.String("error", err.Error()))
	}

	return p, nil
}

// ensureBucket membuat bucket bila belum ada.
func (p *Photos) ensureBucket(ctx context.Context) error {
	exists, err := p.client.BucketExists(ctx, p.cfg.Bucket)
	if err != nil {
		return fmt.Errorf("memeriksa bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := p.client.MakeBucket(ctx, p.cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("membuat bucket: %w", err)
	}
	p.logger.InfoContext(ctx, "bucket foto resep dibuat", slog.String("bucket", p.cfg.Bucket))
	return nil
}

// ErrStorageTidakSiap = penyimpanan ada tapi bucket-nya belum siap.
var ErrStorageTidakSiap = errors.New("penyimpanan foto belum siap")

// Upload menyimpan satu foto resep dan mengembalikan kunci objeknya.
//
// Kunci objek, BUKAN URL, yang disimpan ke database. Alasannya: bucket-nya
// privat, jadi URL apa pun akan kedaluwarsa. Menyimpan URL berarti kolomnya
// berisi tautan mati dalam hitungan jam; menyimpan kunci membuat tautannya
// bisa dibuat ulang kapan saja.
func (p *Photos) Upload(ctx context.Context, ownerID, medicationID string, data []byte, contentType string) (string, error) {
	if p == nil {
		return "", ErrStorageTidakSiap
	}

	ct := normalizeContentType(contentType)
	if ct == "" {
		return "", fmt.Errorf("%w: %s", ErrPhotoTypeTidakDidukung, contentType)
	}
	if len(data) > MaxPhotoBytes {
		return "", fmt.Errorf("%w: maksimum %d MiB", ErrPhotoTooLarge, MaxPhotoBytes>>20)
	}
	if len(data) == 0 {
		return "", errors.New("berkas kosong")
	}

	// Nama objek memuat ID pemilik dan ID obat, sehingga satu berkas bisa
	// ditelusuri ke pemiliknya tanpa membaca database. Ini penting saat
	// menangani penghapusan data pengguna: berkas harus bisa ditemukan
	// lewat awalan namanya.
	ext := extensionFor(ct)
	key := path.Join("photos", ownerID, medicationID, fmt.Sprintf("%d%s", time.Now().UnixNano(), ext))

	_, err := p.client.PutObject(ctx, p.cfg.Bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: ct})
	if err != nil {
		return "", fmt.Errorf("menyimpan foto: %w", err)
	}
	return key, nil
}

// PresignedURL membuat tautan sementara untuk membaca satu foto.
//
// Masa berlakunya pendek (10 menit) karena ini DATA KESEHATAN: tautan yang
// bocor — lewat riwayat peramban, tangkapan layar, atau log proxy — tidak
// boleh tetap bisa dibuka berhari-hari. Pengguna yang masih membuka halaman
// akan meminta tautan baru.
func (p *Photos) PresignedURL(ctx context.Context, key string) (string, error) {
	if p == nil {
		return "", ErrStorageTidakSiap
	}
	if key == "" {
		return "", errors.New("kunci objek kosong")
	}
	u, err := p.client.PresignedGetObject(ctx, p.cfg.Bucket, key, 10*time.Minute, nil)
	if err != nil {
		return "", fmt.Errorf("membuat tautan foto: %w", err)
	}
	return u.String(), nil
}

// Read membaca isi objek. Dipakai server web untuk menyajikan foto lewat
// origin yang sama, sehingga CSP `connect-src 'self'` tidak dilanggar dan
// URL penyimpanan tidak perlu dibuka ke publik.
func (p *Photos) Read(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if p == nil {
		return nil, "", ErrStorageTidakSiap
	}
	obj, err := p.client.GetObject(ctx, p.cfg.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("membaca foto: %w", err)
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, "", fmt.Errorf("membaca keterangan foto: %w", err)
	}
	return obj, info.ContentType, nil
}

// Delete membuang satu objek foto.
func (p *Photos) Delete(ctx context.Context, key string) error {
	if p == nil || key == "" {
		return nil
	}
	if err := p.client.RemoveObject(ctx, p.cfg.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("menghapus foto: %w", err)
	}
	return nil
}

// normalizeContentType menerima hanya jenis gambar yang lazim dari kamera
// ponsel, dan mengembalikan string kosong untuk yang lain.
//
// Daftar putih, bukan daftar hitam: yang tidak diizinkan adalah default.
// Aplikasi ini menyajikan kembali berkas yang diunggah, jadi menerima
// sembarang jenis berkas berarti menyiapkan cara bagi penyerang menaruh
// konten berbahaya di domain yang sama.
func normalizeContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/png":
		return "image/png"
	case "image/webp":
		return "image/webp"
	case "image/heic", "image/heif":
		return "image/heic"
	default:
		return ""
	}
}

// extensionFor memetakan jenis konten ke akhiran berkas.
func extensionFor(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	default:
		return ".bin"
	}
}
