# Build stage
FROM registry.access.redhat.com/ubi9/go-toolset:1.25.5 AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
USER root
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o kubevirt-service-provider ./cmd/kubevirt-service-provider

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

WORKDIR /app

COPY --from=builder /app/kubevirt-service-provider .

EXPOSE 8080

ENTRYPOINT ["./kubevirt-service-provider"]
