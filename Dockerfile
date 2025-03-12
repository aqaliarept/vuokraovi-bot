# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bot

# Final stage
FROM alpine:3.19

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/bot .

# Create a non-root user
RUN adduser -D -g '' appuser && \
    chown -R appuser:appuser /app

USER appuser

# Command to run the application
ENTRYPOINT ["/app/bot"]
CMD ["-bot"] 