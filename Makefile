# =============================================================================
# Makefile — Pantau Obat
#
# Titik masuk tunggal untuk pekerjaan sehari-hari. Jalankan `make` atau
# `make help` untuk melihat semua target.
#
# Platform:
#   - macOS dan Linux Debian-based: didukung langsung.
#   - Windows: pakai WSL2. Makefile ini tidak berjalan di Windows native.
# =============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

# ---- Versi yang dipatok -----------------------------------------------------
GO_VERSION      ?= 1.27.2
NODE_MAJOR      ?= 24

# ---- Lokasi -----------------------------------------------------------------
ROOT            := $(CURDIR)
DB_DIR          := $(ROOT)/db
DEPLOY_DIR      := $(ROOT)/deploy
SCRIPTS_DIR     := $(ROOT)/scripts

# Binary Go user-local (hasil scripts/setup-toolchain.sh)
GOROOT_LOCAL    := $(HOME)/.local/opt/go
GO_BIN          := $(GOROOT_LOCAL)/bin/go
GOPATH_LOCAL    := $(HOME)/go

# Migrasi goose
MIG_DIR         := backend/migrations
GOOSE           := $(GOPATH_LOCAL)/bin/goose
GOOSE_DRIVER    ?= postgres
# Kredensial ikut deploy/.env; default menunjuk ke container lokal.
GOOSE_DBSTRING  ?= postgres://$(DB_USER):$(POSTGRES_PASSWORD)@127.0.0.1:$(or $(POSTGRES_PORT),5432)/$(DB_NAME)?sslmode=disable
export GOOSE_DBSTRING

# Compose: hormati `docker compose` (v2) dan jatuh ke `docker-compose` (v1).
COMPOSE         := $(shell if docker compose version >/dev/null 2>&1; then echo "docker compose"; else echo "docker-compose"; fi)
COMPOSE_FILE    := $(DEPLOY_DIR)/docker-compose.yml

# Environment database (dibaca dari deploy/.env bila ada)
-include $(DEPLOY_DIR)/.env
export
DB_NAME         ?= pantau_obat
DB_USER         ?= pantau_obat
PG_SERVICE      := postgres

# ---- Warna ------------------------------------------------------------------
ifneq (,$(findstring xterm,$(TERM)))
  BOLD  := $(shell tput bold)
  DIM   := $(shell tput dim)
  GREEN := $(shell tput setaf 2)
  BLUE  := $(shell tput setaf 4)
  YELLOW:= $(shell tput setaf 3)
  RED   := $(shell tput setaf 1)
  RESET := $(shell tput sgr0)
endif

# =============================================================================
# Bantuan
# =============================================================================
.PHONY: help
help: ## Tampilkan daftar target
	@printf '%sPantau Obat — perintah tersedia%s\n\n' '$(BOLD)$(BLUE)' '$(RESET)'
	@grep -hE '^[a-zA-Z0-9_.-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-22s$(RESET) %s\n", $$1, $$2}'
	@printf '\n%sVersi dipatok:%s Go %s, Node %s\n\n' '$(DIM)' '$(RESET)' '$(GO_VERSION)' '$(NODE_MAJOR)'

# =============================================================================
# Toolchain
# =============================================================================
.PHONY: setup-toolchain check-tools
setup-toolchain: ## Pasang Go + nvm + Node + pnpm + TypeScript (user-local)
	@bash $(SCRIPTS_DIR)/setup-toolchain.sh

check-tools: ## Periksa toolchain tanpa mengubah apa pun
	@bash $(SCRIPTS_DIR)/setup-toolchain.sh --check

# =============================================================================
# Database
# =============================================================================
.PHONY: db-up db-down db-reset db-migrate db-rollback db-hard-reset db-status db-new db-shell db-ps db-logs db-backup db-restore
db-up: ## Nyalakan PostgreSQL 18 (container)
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) up -d
	@printf '%spostsgres siap. cek dengan: make db-ps%s\n' '$(GREEN)' '$(RESET)'

db-down: ## Matikan container (data tetap)
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) down

db-reset: ## HATI-HATI: hapus volume lalu nyalakan ulang dari nol
	@printf '%sMenghapus volume database...%s\n' '$(RED)$(BOLD)' '$(RESET)'
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) down -v
	@$(MAKE) db-up
	@$(MAKE) db-migrate

db-migrate: ## Jalankan migrasi goose yang belum naik
	@printf 'Menjalankan migrasi dari %s...\n' '$(MIG_DIR)'
	@cd $(ROOT) && GOOSE_DRIVER=$(GOOSE_DRIVER) $(GOOSE) -dir $(MIG_DIR) up
	@$(MAKE) --no-print-directory db-status

db-rollback: ## Turunkan migrasi. Pakai: make db-rollback N=3
	@cd $(ROOT) && GOOSE_DRIVER=$(GOOSE_DRIVER) $(GOOSE) -dir $(MIG_DIR) down $(or $(N),1)

db-hard-reset: ## HATI-HATI: rollback semua migrasi lalu naik lagi dari nol
	@printf '%sRollback penuh lalu migrasi ulang. SEMUA DATA HILANG.%s\n' '$(RED)$(BOLD)' '$(RESET)'
	@cd $(ROOT) && GOOSE_DRIVER=$(GOOSE_DRIVER) $(GOOSE) -dir $(MIG_DIR) down-to 0
	@$(MAKE) --no-print-directory db-migrate

db-status: ## Status migrasi: mana yang sudah dan belum jalan
	@cd $(ROOT) && GOOSE_DRIVER=$(GOOSE_DRIVER) $(GOOSE) -dir $(MIG_DIR) status

db-new: ## Buat file migrasi baru. Pakai: make db-new NAME=tambah_kolom_x
	@test -n "$(NAME)" || { printf '%sPakai: make db-new NAME=nama_perubahan%s\n' '$(RED)' '$(RESET)'; exit 1; }
	@cd $(ROOT) && GOOSE_DRIVER=$(GOOSE_DRIVER) $(GOOSE) -dir $(MIG_DIR) create $(NAME) sql
	@printf '%sFile dibuat. Isi Up dan Down. Untuk fungsi plpgsql wajib pakai StatementBegin/End.%s\n' '$(DIM)' '$(RESET)'

db-ps: ## Status container database
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) ps

db-logs: ## Ikuti log PostgreSQL
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) logs -f $(PG_SERVICE)

db-shell: ## Buka psql interaktif
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) exec $(PG_SERVICE) \
		psql -U $(DB_USER) -d $(DB_NAME)

db-backup: ## Dump database ke deploy/backup/
	@mkdir -p $(DEPLOY_DIR)/backup
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) exec -T $(PG_SERVICE) \
		pg_dump -U $(DB_USER) -Fc $(DB_NAME) \
		> $(DEPLOY_DIR)/backup/$(DB_NAME)-$$(date +%Y%m%d-%H%M%S).dump
	@printf '%sbackup selesai.%s\n' '$(GREEN)' '$(RESET)'

db-restore: ## Pulihkan dari FILE=path/to/file.dump
	@test -n "$(FILE)" || { printf '%sPakai: make db-restore FILE=deploy/backup/xxx.dump%s\n' '$(RED)' '$(RESET)'; exit 1; }
	@cat $(FILE) | $(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) exec -T $(PG_SERVICE) \
		pg_restore -U $(DB_USER) -d $(DB_NAME) --clean --if-exists
	@printf '%srestore selesai.%s\n' '$(GREEN)' '$(RESET)'

# =============================================================================
# Backend (aktif setelah backend/ diisi)
# =============================================================================
.PHONY: be-build be-run be-test be-lint be-tidy be-migrate be-clean
BE_DIR := $(ROOT)/backend

be-build: ## Build binary backend ke backend/bin/api
	@test -f $(BE_DIR)/go.mod || { printf '%sbackend/ belum ada go.mod%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@cd $(BE_DIR) && $(GO_BIN) build -o bin/api ./cmd/api
	@printf '%sbackend/bin/api siap.%s\n' '$(GREEN)' '$(RESET)'

be-run: ## Jalankan backend (go run)
	@test -f $(BE_DIR)/go.mod || { printf '%sbackend/ belum ada go.mod%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@cd $(BE_DIR) && $(GO_BIN) run ./cmd/api

be-test: ## Test backend (unit + integration)
	@test -f $(BE_DIR)/go.mod || { printf '%sbackend/ belum ada go.mod%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@cd $(BE_DIR) && $(GO_BIN) test ./... -count=1

be-lint: ## Vet + gofmt check backend
	@test -f $(BE_DIR)/go.mod || { printf '%sbackend/ belum ada go.mod%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@cd $(BE_DIR) && $(GO_BIN) vet ./...
	@cd $(BE_DIR) && test -z "$$($(GO_BIN) fmt ./...)" || { printf '%sgo fmt mengubah file, jalankan ulang.%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }

be-tidy: ## go mod tidy
	@cd $(BE_DIR) && $(GO_BIN) mod tidy

be-migrate: ## Jalankan migrasi goose (alias dari db-migrate)
	@$(MAKE) --no-print-directory db-migrate

be-clean: ## Bersihkan artefak build backend
	@rm -rf $(BE_DIR)/bin

# =============================================================================
# Frontend (aktif setelah frontend/ diisi)
# =============================================================================
.PHONY: fe-install fe-dev-user fe-build fe-test fe-check fe-clean
FE_DIR := $(ROOT)/frontend

fe-install: ## Pasang dependency semua workspace frontend
	@test -f $(FE_DIR)/package.json || { printf '%sfrontend/ belum ada package.json%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@cd $(FE_DIR) && pnpm install

fe-dev-user: ## Dev server aplikasi web (landing + user)
	@cd $(FE_DIR) && pnpm --filter @pantau-obat/web dev

fe-build: ## Build produksi aplikasi web (statis)
	@cd $(FE_DIR) && pnpm -r build
	@printf '%sbuild frontend selesai.%s\n' '$(GREEN)' '$(RESET)'

fe-test: ## Test frontend (vitest)
	@cd $(FE_DIR)/apps/web && pnpm test

fe-check: ## svelte-check + tsc
	@cd $(FE_DIR)/apps/web && pnpm check

fe-clean: ## Bersihkan artefak build frontend
	@rm -rf $(FE_DIR)/apps/*/.svelte-kit $(FE_DIR)/apps/*/build $(FE_DIR)/packages/*/dist

# =============================================================================
# API Documentation / E2E (OpenCollection)
# =============================================================================
.PHONY: apidoc-install apidoc-run apidoc-check
APIDOC_DIR   := $(ROOT)/apidoc
BRU          ?= npx --yes @usebruno/cli@latest
ADOC_ENV     ?= local

apidoc-install: ## Pasang bruno CLI (bru) secara lokal agar tidak unduh tiap run
	@printf 'Memasang @usebruno/cli...\n'
	@npm install -g @usebruno/cli@latest
	@command -v bru >/dev/null && bru --version || printf '%sbru tidak ditemukan di PATH%s\n' '$(YELLOW)' '$(RESET)'

apidoc-check: ## Periksa kelengkapan file apidoc tanpa menjalankan
	@test -d $(APIDOC_DIR) || { printf '%sapidoc/ belum ada%s\n' '$(RED)' '$(RESET)'; exit 1; }
	@printf 'File request di apidoc/: '
	@find $(APIDOC_DIR) -name '*.yml' -not -path '*/environments/*' -not -name 'opencollection.yml' | wc -l
	@test -f $(APIDOC_DIR)/opencollection.yml || printf '%sbelum ada opencollection.yml%s\n' '$(YELLOW)' '$(RESET)'

apidoc-run: ## Jalankan koleksi apidoc sebagai uji e2e. Pakai: make apidoc-run [ADOC_ENV=local]
	@test -d $(APIDOC_DIR) || { printf '%sapidoc/ belum ada%s\n' '$(RED)' '$(RESET)'; exit 1; }
	@test -f $(APIDOC_DIR)/opencollection.yml || { printf '%sopencollection.yml belum ada. Lihat apidoc/README.md%s\n' '$(YELLOW)' '$(RESET)'; exit 1; }
	@printf '%sMenjalankan koleksi apidoc (environment: %s)...%s\n' '$(BOLD)$(BLUE)' '$(ADOC_ENV)' '$(RESET)'
	@cd $(APIDOC_DIR) && $(BRU) run --env $(ADOC_ENV)

# =============================================================================
# Pemeriksaan menyeluruh
# =============================================================================
.PHONY: verify
verify: check-tools db-ps ## Verifikasi menyeluruh: toolchain + DB + skema
	@printf '\n%s== Verifikasi ==%s\n' '$(BOLD)$(BLUE)' '$(RESET)'
	@printf 'Skema di database:\n'
	@$(COMPOSE) -f $(COMPOSE_FILE) --project-directory $(DEPLOY_DIR) exec -T $(PG_SERVICE) \
		psql -U $(DB_USER) -d $(DB_NAME) -Atc \
		"SELECT '  tabel = '||count(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' UNION ALL SELECT '  view  = '||count(*) FROM information_schema.views WHERE table_schema='public';"
	@printf '\n%sStatus migrasi:%s\n' '$(DIM)' '$(RESET)'
	@$(MAKE) --no-print-directory db-status 2>/dev/null || printf '  (goose belum terpasang: make setup-toolchain)\n'

.PHONY: clean
clean: be-clean fe-clean ## Bersihkan semua artefak build
	@printf '%sbersih.%s\n' '$(GREEN)' '$(RESET)'
