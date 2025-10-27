# Build stage
FROM golang:1.23-alpine AS build-env

# Set the working directory
WORKDIR /build

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main ./cmd/nodes/main.go

# Final stage
FROM alpine:latest

# Set the working directory
WORKDIR /app/

# Install necessary packages
RUN apk add --no-cache netcat-openbsd

# Copy the binary from the build stage
COPY --from=build-env /build/main ./
COPY --from=build-env /build/static-key/leadernode.bin ./static-key/leadernode.bin

# Copy the migration file
COPY database/migrations /app/migrations

# Copy the ABI files
COPY contract/abi/Commit2RevealDRB.json /app/contract/abi/Commit2RevealDRB.json

# Ensure the binary is executable
RUN chmod +x ./main

# Run the binary
CMD ["./main"]
