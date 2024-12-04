# Use Go 1.23 bookworm as base image
FROM golang:1.23-bookworm AS base

# Builder stage
# =============================================================================
# Create a builder stage based on the "base" image
FROM base AS builder

# Move to working directory /build
WORKDIR /build

RUN apt-get update && apt-get install libaom-dev -y --no-install-recommends

# Copy the go.mod and go.sum files to the /build directory
COPY go.mod go.sum ./

# Install dependencies
RUN go mod download

# Copy the entire source code into the container
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -o demo-app

# Production stage
# =============================================================================
# Create a production stage to run the application binary
FROM scratch AS production

# Move to working directory /prod
WORKDIR /prod

# Copy binary from builder stage
COPY --from=builder /build/demo-app ./

# Document the port that may need to be published
EXPOSE 9113

# Start the application
CMD ["/prod/demo-app"]
