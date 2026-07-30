# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o incipit .

# Runtime stage — scratch image, just the binary
FROM scratch
COPY --from=builder /app/incipit /incipit

ENTRYPOINT ["/incipit"]
CMD ["serve"]