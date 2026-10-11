#!/usr/bin/env python3
"""
Sisipkan penanda goose Up/Down ke setiap file migrasi.

Format yang dihasilkan:

    -- +goose Up
    <isi migrasi>

    -- +goose Down
    DROP ... (kebalikan, terurut terbalik)

Blok Down dibangkitkan dari isi file. Hanya pernyataan yang bisa dibalik
secara mekanis yang ditangani: CREATE TABLE, CREATE INDEX, CREATE TRIGGER,
CREATE VIEW, CREATE FUNCTION, CREATE EXTENSION, ALTER TABLE ... ADD
COLUMN/CONSTRAINT, dan COMMENT ON.

Kalau ada pernyataan yang tidak dikenali, skrip BERHENTI dan meminta Down
ditulis manual. Lebih baik gagal jelas daripada menghasilkan Down yang salah.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

MIGRATIONS = Path(__file__).resolve().parent.parent / "backend" / "migrations"

# Urutan penting: pernyataan yang lebih spesifik diperiksa lebih dulu.
PATTERNS = [
    (re.compile(r"CREATE TABLE\s+(?:IF NOT EXISTS\s+)?([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP TABLE IF EXISTS {m.group(1)} CASCADE;"),
    (re.compile(r"CREATE UNIQUE INDEX\s+(?:IF NOT EXISTS\s+)?([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP INDEX IF EXISTS {m.group(1)};"),
    (re.compile(r"CREATE INDEX\s+(?:IF NOT EXISTS\s+)?([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP INDEX IF EXISTS {m.group(1)};"),
    (re.compile(r"CREATE TRIGGER\s+([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP TRIGGER IF EXISTS {m.group(1)} ON {{table}};"),
    (re.compile(r"CREATE OR REPLACE VIEW\s+([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP VIEW IF EXISTS {m.group(1)} CASCADE;"),
    (re.compile(r"CREATE OR REPLACE FUNCTION\s+([a-zA-Z_][\w.]*)", re.I),
     lambda m: f"DROP FUNCTION IF EXISTS {m.group(1)}();"),
    (re.compile(r"CREATE EXTENSION\s+(?:IF NOT EXISTS\s+)?([a-zA-Z_][\w]*)", re.I),
     lambda m: f"DROP EXTENSION IF EXISTS {m.group(1)};"),
    (re.compile(r"ALTER TABLE\s+([a-zA-Z_][\w.]*)\s+ADD COLUMN\s+(?:IF NOT EXISTS\s+)?([a-zA-Z_]\w*)", re.I),
     lambda m: f"ALTER TABLE {m.group(1)} DROP COLUMN IF EXISTS {m.group(2)};"),
    (re.compile(r"ALTER TABLE\s+([a-zA-Z_][\w.]*)\s+ADD CONSTRAINT\s+([a-zA-Z_]\w*)", re.I),
     lambda m: f"ALTER TABLE {m.group(1)} DROP CONSTRAINT IF EXISTS {m.group(2)};"),
    (re.compile(r"COMMENT ON\s+(?:TABLE|COLUMN|VIEW|FUNCTION|SCHEMA)\s+([^\s]+)", re.I),
     lambda m: None),  # komentar tidak perlu dibalik
]

SKIP = re.compile(r"^\s*(?:--.*)?$")

# Baris pemisah bagian dan blok komentar yang berdiri sendiri: bukan pernyataan.
SEPARATOR = re.compile(r"^\s*--[\s-]*$")
COMMENT_LINE = re.compile(r"^\s*--")


def strip_noise(text: str) -> str:
    """Buang baris pemisah bagian dan komentar yang berdiri sendiri.

    Komentar di dalam pernyataan (setelah kode) tetap dipertahankan supaya
    konteksnya tidak hilang.
    """
    out: list[str] = []
    for line in text.split("\n"):
        if SEPARATOR.match(line):
            continue
        out.append(line)
    return "\n".join(out)


def split_statements(sql: str) -> list[str]:
    """Pecah SQL jadi pernyataan, sadar dolar-quote dan string literal."""
    stmts: list[str] = []
    buf: list[str] = []
    i, n = 0, len(sql)
    dollar = None
    in_str = False

    while i < n:
        ch = sql[i]

        if dollar:
            if sql.startswith(dollar, i):
                buf.append(dollar)
                i += len(dollar)
                dollar = None
                continue
            buf.append(ch)
            i += 1
            continue

        if in_str:
            buf.append(ch)
            if ch == "'":
                if i + 1 < n and sql[i + 1] == "'":
                    buf.append("'")
                    i += 2
                    continue
                in_str = False
            i += 1
            continue

        if ch == "'":
            in_str = True
            buf.append(ch)
            i += 1
            continue

        if ch == "$":
            m = re.match(r"\$[A-Za-z_]*\$", sql[i:])
            if m:
                dollar = m.group(0)
                buf.append(dollar)
                i += len(dollar)
                continue

        if ch == ";":
            buf.append(ch)
            stmt = "".join(buf).strip()
            if stmt and not SKIP.match(stmt):
                stmts.append(stmt)
            buf = []
            i += 1
            continue

        buf.append(ch)
        i += 1

    tail = "".join(buf).strip()
    if tail and not SKIP.match(tail):
        stmts.append(tail)

    # Buang pernyataan yang isinya hanya komentar.
    clean: list[str] = []
    for stmt in stmts:
        code = "\n".join(
            l for l in stmt.split("\n") if not COMMENT_LINE.match(l)
        ).strip()
        if not code or code == ";":
            continue
        clean.append(stmt)
    return clean


def trigger_table(stmt: str) -> str | None:
    m = re.search(r"\bON\s+([a-zA-Z_][\w.]*)", stmt, re.I)
    return m.group(1) if m else None


def build_down(stmts: list[str]) -> tuple[list[str], list[str]]:
    down: list[str] = []
    unknown: list[str] = []

    for stmt in reversed(stmts):
        head = " ".join(stmt.split())[:110]
        matched = False

        for rx, maker in PATTERNS:
            m = rx.search(stmt)
            if not m:
                continue
            matched = True
            out = maker(m)
            if out is None:
                break
            if "{table}" in out:
                tbl = trigger_table(stmt)
                if not tbl:
                    unknown.append(head)
                    break
                out = out.replace("{table}", tbl)
            down.append(out)
            break

        if not matched:
            unknown.append(head)

    return down, unknown


def main() -> int:
    if not MIGRATIONS.is_dir():
        print(f"folder tidak ada: {MIGRATIONS}", file=sys.stderr)
        return 1

    files = sorted(MIGRATIONS.glob("*.sql"))
    if not files:
        print(f"tidak ada file .sql di {MIGRATIONS}", file=sys.stderr)
        return 1

    failed: list[tuple[str, list[str]]] = []

    for path in files:
        raw = path.read_text(encoding="utf-8")
        if "+goose Up" in raw:
            print(f"  lewati  {path.name} (sudah punya penanda)")
            continue

        up_body = strip_noise(raw).strip()
        stmts = split_statements(up_body)
        if not stmts:
            print(f"  GAGAL   {path.name}: tidak ada pernyataan setelah pembersihan", file=sys.stderr)
            failed.append((path.name, ["kosong"]))
            continue
        down, unknown = build_down(stmts)

        if unknown:
            failed.append((path.name, unknown))
            print(f"  GAGAL   {path.name}: {len(unknown)} pernyataan tidak dikenali")
            for u in unknown[:3]:
                print(f"            {u}")
            continue

        down_sql = "\n".join(down) if down else "-- tidak ada yang perlu dibalik"
        path.write_text(
            f"-- +goose Up\n{up_body}\n\n-- +goose Down\n{down_sql}\n",
            encoding="utf-8",
        )
        print(f"  ok      {path.name}  (Up {len(stmts)} stmt, Down {len(down)} stmt)")

    if failed:
        print("\nAda file yang butuh Down manual. Perbaiki dulu.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
