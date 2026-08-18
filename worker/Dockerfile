FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-bin ./worker/main.go

# pdfcpu CLI para marcadores de PDF (nivel 1). Requiere Go >= 1.25.
FROM golang:1.25-alpine AS pdfcpu-builder
RUN go env -w GOFLAGS=-mod=mod && \
    go install github.com/pdfcpu/pdfcpu/cmd/pdfcpu@v0.15.0

FROM alpine:3.19

# Utilidades de procesamiento documental para el pipeline de conversión
RUN apk add --no-cache \
    ca-certificates \
    pandoc \
    poppler-utils

WORKDIR /app
COPY --from=builder /worker-bin .
COPY --from=pdfcpu-builder /go/bin/pdfcpu /usr/local/bin/pdfcpu

CMD ["./worker-bin"]