FROM golang:1.24-alpine AS base

FROM base AS builder
WORKDIR /build
COPY go.mod go.sum /build/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /external-dns-bunny

FROM alpine:latest
COPY --from=builder --chown=root:root external-dns-bunny /bin/
USER 65534
CMD ["/bin/external-dns-bunny"]
