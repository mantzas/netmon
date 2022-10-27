FROM golang:1.19 as builder
WORKDIR /app
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -o netmon ./cmd/main.go

FROM alpine:3.16
USER nobody
WORKDIR /app
COPY --from=builder /app/netmon .
CMD ["./netmon"]