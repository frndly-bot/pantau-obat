# =============================================================================
# Build stage — mengompilasi binary statis
# =============================================================================
# CGO_ENABLED=0 menghasilkan binary yang STATIS. Itu bukan preferensi gaya:
# driver pgx murni Go, jadi tidak butuh cgo, dan binary statis bisa berjalan
# di image runtime SEKETEHITUS APAPUN (scratch, distroless, alpine) tanpa
# membawa pustaka C yang versinya bisa berbeda antara build dan runtime.
#
# go.mod dan go.sum disalin lebih dulu, sebelum kode. Ini memanfaatkan layer
# cache Docker: dependency hanya diunduh ulang kalau go.mod berubah — bukan
# setiap kali satu file .go disentuh.
FROM golang:1.27.2-alpine AS build

WORKDIR /src

# Tambah CA certificates supaya `go mod download` bisa menjangkau proxy
# HTTPS di jaringan yang memakai TLS interception.
#
# tzdata dipasang di BUILD stage untuk DISALIN ke runtime — bukan sekadar
# dipasang di sana. Aplikasi memuat zona waktu (Asia/Jakarta dkk.) saat
# menghitung jadwal pengingat, dan image scratch tidak membawa database
# zona waktu sama sekali.
RUN apk add --no-cache ca-certificates tzdata

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/admin-bootstrap ./cmd/admin-bootstrap

# =============================================================================
# Runtime stage — image sekecil dan sesempit mungkin
# =============================================================================
# scratch = TIDAK ADA shell, TIDAK ADA package manager, TIDAK ADA tool apa pun.
#
# Konsekuensinya disengaja:
#   - Penyerang yang berhasil mengeksekusi kode tidak bisa menjalankan apa pun.
#   - Surface serangan jauh lebih kecil daripada image alpine/debian.
#   - Ukuran image ~15 MB, bukan ~300 MB.
#
# Yang harus disediakan manual karena tidak ada di scratch:
#   - CA certificates (kirim email SMTP/TLS, panggil API eksternal)
#   - zona waktu (jadwal pengingat memakai Asia/Jakarta dkk.)
#   - non-root user (container tidak boleh berjalan sebagai root)
FROM scratch AS runtime

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=build /out/api /api
COPY --from=build /out/admin-bootstrap /admin-bootstrap

# User non-root, UID tetap supaya ownership volume bisa diprediksi.
# Tidak ada HOME dan tidak ada shell: aplikasi tidak butuh keduanya.
USER 10001:10001

# Healthcheck memakai binary api sendiri: /api healthz melakukan HTTP GET
# ke endpoint liveness. Tanpa shell dan tanpa curl di scratch, ini satu-satunya
# cara yang tersisa — dan justru yang paling akurat, karena yang diperiksa
# adalah endpoint yang sama dengan yang dipakai Docker/Caddy.
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=3 \
    CMD ["/api", "healthz"]

ENTRYPOINT ["/api"]
