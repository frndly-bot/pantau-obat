# Pantau Obat — Personal Medication Adherence + Caregiver Monitoring

Status: **backend is running & tested.** 77 routes, 92 unit tests passing, and the
apidoc e2e suite **75/75 requests with 159/159 assertions** (idempotent). The
frontend has not started; HTML wireframes live in `design/`.
Stack: Backend Go 1.27 | DB PostgreSQL 18 | Frontend SvelteKit + Svelte 5 + shadcn-svelte | PWA (native later)

## Layout

```
pantau-obat/
  Makefile               # single entry point for every command
  scripts/
    setup-toolchain.sh   # install Go + Node + pnpm + TypeScript
    dev-api.sh           # run the backend locally (loads env, sets APP_PUBLIC_URL)
  db/
    README.md            # goose migration rules
  backend/
    migrations -> internal/repo/postgres/migrations   # 25 migrations (symlink)
    cmd/ internal/       # Go 1.27, single binary (see README)
    README.md
  apidoc/                # OpenCollection spec + API e2e tests (75 endpoints)
    README.md
  deploy/
    docker-compose.yml   # PostgreSQL 18, resource-efficient tuning
    .env.example         # example configuration
    postgres/            # postgresql.conf + initdb
    README.md
  design/                # interactive HTML wireframes (user + admin)
  frontend/              # EMPTY — SvelteKit workspace (see README)
  docs/
    trd-v2/              # TRD v2
    design-v2/           # architecture, data model, API, notifications
    decisions-v2/        # ADR 001-005
    design/erd.html      # visual ERD (open directly in a browser)
    issues/              # per-issue debug notes
```

## Running the backend

```bash
make db-up                    # start PostgreSQL 18
make db-migrate               # run goose migrations (25 files)
scripts/dev-api.sh --bg       # run the API in the background, log to /tmp
curl -s localhost:8080/readyz # confirm database: ok

# first admin (the admin role cannot be granted through the API)
cd backend && go run ./cmd/admin-bootstrap \
    --email admin@example.com --password 'ChangeMe123!' --name "Admin"

make apidoc-run               # e2e test: 75 requests, 159 assertions
```

## Active documents

### TRD
- `docs/trd-v2/TRD-v2.md` — goals, personas, user stories, AC, NFR, version scope

### Design
- `docs/design-v2/architecture-v2.md` — architecture, backend/frontend structure, deployment
- `docs/design-v2/data-model-v2.md` — full database entities + rationale for each index
- `docs/design-v2/api-v2.md` — endpoint contract, error codes, envelope
- `docs/design-v2/notification-v2.md` — reminder strategy, queue, missed doses
- `docs/design/erd.html` — visual ERD, offline, open in a browser

### Decisions
- `docs/decisions-v2/ADR-001-stack-v2.md` — overall stack choice
- `docs/decisions-v2/ADR-002-postgresql-18-v2.md` — PostgreSQL 18, uuidv7, tuning
- `docs/decisions-v2/ADR-003-frontend-dua-app.md` — why two frontend apps
- `docs/decisions-v2/ADR-004-katalog-preset.md` — public presets vs the user's private catalog
- `docs/decisions-v2/ADR-005-jadwal-aturan.md` — why schedules are stored as rules

## Principles

- **Explore → plan → build.** No code before the plan is approved.
- **Wireframes = interactive HTML, one page per file in a single folder**, not ASCII.
- **Mobile responsive = a requirement**, not polish.
- **`backend/migrations/` is the source of truth for the schema.** If a document disagrees, the migrations win.
- **Database enums are in English** for maintainability; translated to Indonesian in the frontend.
- **Verify live, not just green unit tests.** Evidence from the running container.

## Major changes from v1

1. **Schedules are stored as rules**, not one row per day. Lifelong therapy stays at 3 rows.
2. **Frequency patterns expanded** to five forms: daily, interval, weekly, monthly cycle, PRN.
3. **Stock and restock** added, including low-medication warnings.
4. **Drug catalog** with public presets (admin-curated) separated from each user's private catalog.
5. **Admin role** added, separate from account active status, with `admin_scope`.
6. **Frontend split in two**: user mobile-first and admin desktop-first.
7. **Backend stays one folder and one binary** — admin separation lives in the authorization layer.

## Setting up the environment

**Supported:** macOS (Intel and Apple Silicon) and Debian-based Linux
(Ubuntu, Debian, Linux Mint, Pop!_OS).

**Windows:** not supported directly. Use WSL2 first:

```powershell
wsl --install -d Ubuntu
```

Restart, open Ubuntu, then run the commands below from inside WSL.

```bash
make setup-toolchain   # Go 1.27 + nvm + Node 24 LTS + pnpm + TypeScript
make check-tools       # only checks, changes nothing
```

Everything installs **user-local** under `$HOME`, without sudo.

## Running the database

```bash
make db-up         # start PostgreSQL 18
make db-migrate    # run goose migrations
make verify        # check toolchain + DB + table count
```

Details are in `deploy/README.md`. To see every command:

```bash
make help
```
