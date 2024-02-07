FROM golang:1.22 as builder
RUN update-ca-certificates
WORKDIR /app
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -o netmon ./cmd/main.go

FROM bitnami/minideb:stretch
WORKDIR /app
COPY --from=builder /app/netmon .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
CMD ["./netmon"]