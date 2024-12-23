# Build stage
FROM golang:1.22-alpine AS build-env

# Set environment variables
ENV CONFIG_BASE_PATH /root/

# Set the working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main ./cmd/main.go

# Final stage
FROM alpine:latest

# Set the working directory
WORKDIR /root/

# Install necessary packages
RUN apk --no-cache add ca-certificates

# Copy the binary from the build stage
COPY --from=build-env /app/main .

# Copy the .env file
COPY .env .

# Copy the ABI files
COPY contract/abi/Commit2RevealDRB.json /root/contract/abi/Commit2RevealDRB.json

# Ensure the binary is executable
RUN chmod +x ./main

# Run the binary
CMD ["./main"]
