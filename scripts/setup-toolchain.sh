#!/usr/bin/env bash
# =============================================================================
# scripts/setup-toolchain.sh
# Memasang toolchain Pantau Obat: Go, nvm + Node, pnpm, TypeScript.
#
# Dukungan platform:
#   - macOS          (Intel dan Apple Silicon)
#   - Linux Debian-based (Ubuntu, Debian, Linux Mint, Pop!_OS)
#   - Windows        -> JANGAN jalankan ini. Pakai WSL2 dulu (lihat bawah).
#
# Sifat instalasi: USER-LOCAL. Tidak perlu sudo, tidak mengubah sistem.
# Semua masuk ke $HOME. Kalau versi Go/Node sudah ada, dipakai ulang.
#
# Pemakaian:
#   bash scripts/setup-toolchain.sh           # pasang yang belum ada
#   bash scripts/setup-toolchain.sh --force   # pasang ulang semua
#   bash scripts/setup-toolchain.sh --check   # hanya periksa, tidak mengubah
#
# ==== Windows ====
# Script ini tidak mendukung Windows native. Pakai WSL2:
#   1. Buka PowerShell sebagai Administrator
#   2. wsl --install -d Ubuntu
#   3. Restart, buka Ubuntu, lalu jalankan script ini di dalam WSL
# Referensi: https://learn.microsoft.com/windows/wsl/install
# =============================================================================
set -euo pipefail

# ---- Versi yang dipatok -----------------------------------------------------
GO_VERSION="1.27.2"
NODE_MAJOR="24"          # LTS "Krypton"; current v26 belum LTS
NVM_VERSION="v0.40.3"
# pnpm dan typescript memakai "@latest" supaya selalu versi stabil terbaru.

# ---- Warna ------------------------------------------------------------------
if [ -t 1 ]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BLUE=$'\033[34m'
else
  C_RESET=; C_BOLD=; C_DIM=; C_RED=; C_GREEN=; C_YELLOW=; C_BLUE=
fi

info()  { printf '%s==>%s %s\n' "$C_BLUE$C_BOLD" "$C_RESET" "$*"; }
ok()    { printf '%s ok %s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn()  { printf '%swarn%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
die()   { printf '%serr %s %s\n' "$C_RED" "$C_RESET" "$*" >&2; exit 1; }

# ---- Argumen ----------------------------------------------------------------
FORCE=0
CHECK_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --force)  FORCE=1 ;;
    --check)  CHECK_ONLY=1 ;;
    -h|--help)
      sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *) die "argumen tidak dikenal: $arg (pakai --help)" ;;
  esac
done

# ---- Deteksi platform -------------------------------------------------------
detect_platform() {
  local uname_s
  uname_s="$(uname -s 2>/dev/null || echo unknown)"
  case "$uname_s" in
    Darwin*) PLATFORM="macos" ;;
    Linux*)
      if grep -qi microsoft /proc/version 2>/dev/null; then
        PLATFORM="wsl"
      else
        PLATFORM="linux"
      fi ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT*)
      printf '\n'
      die "Windows native tidak didukung.\n
      Pakai WSL2 dulu, lalu jalankan script ini di dalamnya:\n
        1. PowerShell (Administrator):  wsl --install -d Ubuntu\n
        2. Restart komputer, buka Ubuntu\n
        3. Jalankan ulang: bash scripts/setup-toolchain.sh\n
      Panduan: https://learn.microsoft.com/windows/wsl/install\n" ;;
    *) die "platform tidak dikenal: $uname_s" ;;
  esac

  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64) GO_ARCH="amd64" ;;
    arm64|aarch64) GO_ARCH="arm64" ;;
    *) die "arsitektur tidak didukung: $ARCH" ;;
  esac
}

# Linux Debian-based: butuh paket dasar sebelum bisa memasang Go.
require_base_linux() {
  [ "$PLATFORM" = "linux" ] || return 0
  local missing=()
  for c in curl tar git; do
    command -v "$c" >/dev/null 2>&1 || missing+=("$c")
  done
  if [ "${#missing[@]}" -gt 0 ]; then
    warn "paket dasar belum ada: ${missing[*]}"
    info "pasang dengan (butuh sudo, akan minta password):"
    printf '    sudo apt-get update && sudo apt-get install -y %s\n' "${missing[*]}"
    die "pasang dulu paket dasar di atas, lalu jalankan script ini lagi."
  fi
}

# Menambahkan blok ke file rc, idempoten.
ensure_rc_block() {
  local rcfile="$1" marker="$2" content="$3"
  [ -f "$rcfile" ] || touch "$rcfile"
  if grep -qF ">>> $marker" "$rcfile" 2>/dev/null; then
    return 0
  fi
  {
    printf '\n# >>> %s\n' "$marker"
    printf '%s\n' "$content"
    printf '# <<< %s\n' "$marker"
  } >> "$rcfile"
  ok "blok $marker ditambahkan ke $(basename "$rcfile")"
}

# ---- Go ---------------------------------------------------------------------
install_go() {
  local goroot="$HOME/.local/opt/go"
  local target="$GO_VERSION"

  if [ "$FORCE" -eq 0 ] && [ -x "$goroot/bin/go" ]; then
    local have
    have="$("$goroot/bin/go" version | awk '{print $3}' | sed 's/^go//')"
    if [ "$have" = "$target" ]; then
      ok "Go $have sudah terpasang di $goroot"
      export PATH="$goroot/bin:$PATH"
      return 0
    fi
    warn "Go $have terpasang, target $target — akan diganti"
  fi

  if [ "$CHECK_ONLY" -eq 1 ]; then
    warn "Go $target belum terpasang (mode --check, tidak mengubah apa pun)"
    return 0
  fi

  local tarball="go${target}.linux-${GO_ARCH}.tar.gz"
  [ "$PLATFORM" = "macos" ] && tarball="go${target}.darwin-${GO_ARCH}.tar.gz"
  local url="https://go.dev/dl/${tarball}"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN

  info "unduh Go $target ($PLATFORM/$GO_ARCH)"
  curl -fsSL --retry 3 -o "$tmp/$tarball" "$url" \
    || die "gagal mengunduh $url"

  # Verifikasi checksum resmi. Gagal verifikasi = berhenti.
  local want got
  want="$(curl -fsSL "https://go.dev/dl/?mode=json&include=all" \
          | tr -d ' \n' \
          | grep -o "\"filename\":\"${tarball}\",\"os\":\"[^\"]*\",\"arch\":\"[^\"]*\",\"version\":\"[^\"]*\",\"sha256\":\"[a-f0-9]*\"" \
          | grep -o 'sha256":"[a-f0-9]*' | cut -d'"' -f3 || true)"
  got="$(sha256sum "$tmp/$tarball" 2>/dev/null | awk '{print $1}' \
        || shasum -a 256 "$tmp/$tarball" | awk '{print $1}')"
  if [ -n "$want" ]; then
    [ "$want" = "$got" ] || die "checksum Go tidak cocok!
    harap : $want
    dapat : $got
    Jangan lanjutkan sebelum ini jelas."
    ok "checksum Go terverifikasi"
  else
    warn "checksum resmi tidak bisa diambil, verifikasi dilewati"
  fi

  info "pasang Go $target ke $goroot"
  mkdir -p "$HOME/.local/opt"
  rm -rf "$goroot"
  tar -C "$HOME/.local/opt" -xzf "$tmp/$tarball"
  ok "Go $target terpasang"

  ensure_rc_block "$HOME/.zshrc" "Go toolchain (user-local)" \
    "export GOROOT=\"\$HOME/.local/opt/go\"
export GOPATH=\"\$HOME/go\"
export PATH=\"\$GOROOT/bin:\$GOPATH/bin:\$PATH\""
  ensure_rc_block "$HOME/.bashrc" "Go toolchain (user-local)" \
    "export GOROOT=\"\$HOME/.local/opt/go\"
export GOPATH=\"\$HOME/go\"
export PATH=\"\$GOROOT/bin:\$GOPATH/bin:\$PATH\""

  export GOROOT="$goroot" GOPATH="$HOME/go" PATH="$goroot/bin:$HOME/go/bin:$PATH"
}

# ---- nvm + Node -------------------------------------------------------------
install_node() {
  export NVM_DIR="$HOME/.nvm"

  if [ ! -s "$NVM_DIR/nvm.sh" ]; then
    if [ "$CHECK_ONLY" -eq 1 ]; then
      warn "nvm belum terpasang (mode --check)"
      return 0
    fi
    info "pasang nvm $NVM_VERSION"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "https://raw.githubusercontent.com/nvm-sh/nvm/${NVM_VERSION}/install.sh" | bash
    else
      wget -qO- "https://raw.githubusercontent.com/nvm-sh/nvm/${NVM_VERSION}/install.sh" | bash
    fi
  else
    ok "nvm sudah terpasang"
  fi

  # nvm.sh tidak kompatibel dengan `set -u` karena memakai variabel tak diset.
  set +u
  # shellcheck disable=SC1091
  . "$NVM_DIR/nvm.sh"
  set -u
  ok "nvm $(nvm --version)"

  if [ "$CHECK_ONLY" -eq 1 ]; then
    local cur
    cur="$(nvm version "$NODE_MAJOR" 2>/dev/null || echo none)"
    [ "$cur" = "none" ] && warn "Node $NODE_MAJOR belum terpasang (mode --check)" \
                        || ok "Node $cur tersedia"
    return 0
  fi

  info "pasang Node $NODE_MAJOR (LTS)"
  nvm install "$NODE_MAJOR"
  nvm alias default "$NODE_MAJOR" >/dev/null
  nvm use "$NODE_MAJOR" >/dev/null
  ok "Node $(node --version) aktif, default diarahkan ke $NODE_MAJOR"

  info "pasang pnpm dan TypeScript versi stabil terbaru"
  npm install -g --allow-scripts=pnpm pnpm@latest typescript@latest
  ok "pnpm $(pnpm --version), tsc $(tsc --version)"
}

# ---- Ringkasan --------------------------------------------------------------
summary() {
  set +u
  [ -s "$HOME/.nvm/nvm.sh" ] && . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1
  set -u
  printf '\n%s%sRingkasan%s\n' "$C_BOLD" "$C_BLUE" "$C_RESET"

  local go_bin="$HOME/.local/opt/go/bin/go"
  printf '  %-12s %s\n' "Platform" "$PLATFORM/$ARCH"
  if [ -x "$go_bin" ]; then
    printf '  %-12s %s\n' "Go" "$("$go_bin" version 2>/dev/null | awk '{print $3}')"
  else
    printf '  %-12s %s\n' "Go" "belum"
  fi
  printf '  %-12s %s\n' "Node" "$(node --version 2>/dev/null || echo belum)"
  printf '  %-12s %s\n' "npm" "$(npm --version 2>/dev/null || echo belum)"
  printf '  %-12s %s\n' "pnpm" "$(pnpm --version 2>/dev/null || echo belum)"
  printf '  %-12s %s\n' "TypeScript" "$(tsc --version 2>/dev/null || echo belum)"

  printf '\n%sBuka terminal baru atau jalankan:%s\n' "$C_DIM" "$C_RESET"
  printf '  source ~/.zshrc    # zsh\n'
  printf '  source ~/.bashrc   # bash\n'
  printf '\nLalu verifikasi:  %smake check-tools%s\n\n' "$C_BOLD" "$C_RESET"
}

main() {
  detect_platform
  require_base_linux
  info "platform terdeteksi: $PLATFORM/$ARCH"
  [ "$CHECK_ONLY" -eq 1 ] && info "mode periksa saja (--check)"
  install_go
  install_node
  summary
}

main "$@"
