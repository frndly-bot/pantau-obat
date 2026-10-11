// Package config membaca dan memvalidasi seluruh konfigurasi dari environment.
//
// Aturan: gagal cepat. Variabel wajib yang kosong = log fatal + exit(1).
// Lebih baik proses menolak start daripada jalan dengan konfigurasi separuh.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment adalah mode jalan aplikasi.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Config menampung semua pengaturan runtime. Satu struct, satu sumber.
type Config struct {
	Env       Environment
	Port      string
	PublicURL string

	DB       DBConfig
	Auth     AuthConfig
	Push     PushConfig
	Email    EmailConfig
	Telegram TelegramConfig
	Webhook  WebhookConfig
	Runtime  RuntimeConfig
}

// DBConfig menampung pengaturan koneksi PostgreSQL.
type DBConfig struct {
	URL      string
	MaxConns int32
	MinConns int32
}

// AuthConfig menampung pengaturan token dan password.
type AuthConfig struct {
	JWTSecret     string
	JWTTTL        time.Duration
	RefreshTTL    time.Duration
	Argon2Memory  uint32
	Argon2Time    uint32
	Argon2Threads uint8
}

// PushConfig menampung kunci VAPID untuk web push.
type PushConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// Enabled melaporkan apakah kanal push bisa dipakai.
func (p PushConfig) Enabled() bool {
	return p.PublicKey != "" && p.PrivateKey != "" && p.Subject != ""
}

// EmailConfig menampung pengaturan SMTP (v0.2).
type EmailConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Enabled melaporkan apakah kanal email bisa dipakai.
func (e EmailConfig) Enabled() bool { return e.Host != "" && e.From != "" }

// TelegramConfig menampung pengaturan bot Telegram (v0.2).
type TelegramConfig struct {
	BotToken      string
	WebhookSecret string
}

// Enabled melaporkan apakah kanal telegram bisa dipakai.
func (t TelegramConfig) Enabled() bool { return t.BotToken != "" }

// WebhookConfig menampung secret verifikasi webhook masuk.
type WebhookConfig struct {
	EmailSecret string
}

// RuntimeConfig menampung pengaturan operasional.
type RuntimeConfig struct {
	RateLimitIPPerMin int
	LogLevel          slog.Level
	MigrateOnStart    bool
}

// Load membaca environment dan mengembalikan Config yang sudah divalidasi.
// Mengembalikan error yang menggabungkan SEMUA masalah sekaligus, supaya
// operator tidak memperbaiki satu per satu.
func Load() (*Config, error) {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	env := Environment(getEnv("APP_ENV", string(EnvDevelopment)))
	switch env {
	case EnvDevelopment, EnvStaging, EnvProduction:
	default:
		fail("APP_ENV=%q tidak dikenal (pakai development|staging|production)", env)
	}

	publicURL := getEnv("APP_PUBLIC_URL", "")
	if publicURL == "" {
		fail("APP_PUBLIC_URL wajib diisi")
	}

	dbURL := getEnv("DB_URL", "")
	if dbURL == "" {
		fail("DB_URL wajib diisi")
	}

	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" {
		fail("JWT_SECRET wajib diisi")
	} else if len(jwtSecret) < 32 {
		fail("JWT_SECRET minimal 32 byte (sekarang %d)", len(jwtSecret))
	}
	// Nilai contoh di .env.example jelas tidak boleh dipakai di produksi.
	if env == EnvProduction && strings.HasPrefix(jwtSecret, "ganti_dengan") {
		fail("JWT_SECRET masih memakai nilai contoh; wajib diganti di produksi")
	}

	cfg := &Config{
		Env:       env,
		Port:      getEnv("APP_PORT", "8080"),
		PublicURL: publicURL,
		DB: DBConfig{
			URL:      dbURL,
			MaxConns: int32(getEnvInt("DB_MAX_CONNS", 20)),
			MinConns: int32(getEnvInt("DB_MIN_CONNS", 2)),
		},
		Auth: AuthConfig{
			JWTSecret:  jwtSecret,
			JWTTTL:     time.Duration(getEnvInt("JWT_TTL_MINUTES", 1440)) * time.Minute,
			RefreshTTL: time.Duration(getEnvInt("REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
			// Nilai dari TRD v2 bagian 7.2 dan data-model-v2.
			Argon2Memory:  64 * 1024, // 64 MiB
			Argon2Time:    3,
			Argon2Threads: 4,
		},
		Push: PushConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", ""),
		},
		Email: EmailConfig{
			Host:     getEnv("SMTP_HOST", ""),
			Port:     getEnvInt("SMTP_PORT", 587),
			User:     getEnv("SMTP_USER", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     getEnv("SMTP_FROM", ""),
		},
		Telegram: TelegramConfig{
			BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
			WebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		},
		Webhook: WebhookConfig{
			EmailSecret: getEnv("WEBHOOK_EMAIL_SECRET", ""),
		},
		Runtime: RuntimeConfig{
			RateLimitIPPerMin: getEnvInt("RATE_LIMIT_IP_PER_MIN", 60),
			LogLevel:          parseLogLevel(getEnv("LOG_LEVEL", "info")),
			MigrateOnStart:    getEnvBool("MIGRATE_ON_START", true),
		},
	}

	if cfg.DB.MinConns > cfg.DB.MaxConns {
		fail("DB_MIN_CONNS (%d) tidak boleh melebihi DB_MAX_CONNS (%d)", cfg.DB.MinConns, cfg.DB.MaxConns)
	}

	// Kanal push parsial lebih berbahaya daripada tidak ada: seolah aktif
	// tapi selalu gagal. Tolak sejak awal.
	pushSet := 0
	for _, v := range []string{cfg.Push.PublicKey, cfg.Push.PrivateKey, cfg.Push.Subject} {
		if v != "" {
			pushSet++
		}
	}
	if pushSet != 0 && pushSet != 3 {
		fail("VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY, VAPID_SUBJECT harus diisi ketiganya atau kosong semua")
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("konfigurasi tidak valid:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// IsProduction melaporkan apakah mode produksi aktif.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// Addr mengembalikan alamat listen untuk http.Server.
func (c *Config) Addr() string { return ":" + c.Port }

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func getEnvInt(key string, def int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		// Nilai tak terbaca = pakai default, tapi jangan diam.
		fmt.Fprintf(os.Stderr, "peringatan: %s=%q bukan angka, memakai default %d\n", key, raw, def)
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "peringatan: %s=%q bukan boolean, memakai default %v\n", key, raw, def)
		return def
	}
	return b
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ErrNoEnvFile dikembalikan bila .env tidak ada. Itu bukan kesalahan fatal:
// di produksi, environment biasanya disuntik oleh Docker.
var ErrNoEnvFile = errors.New("file .env tidak ditemukan")
