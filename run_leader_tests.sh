#!/bin/bash

# Start the test database
echo "Starting test database..."
docker compose -f docker-compose.test.yml up -d

# Wait for database to be ready
echo "Waiting for database to be ready..."
until docker exec testdb pg_isready -U postgres > /dev/null 2>&1; do
    echo -n "."
    sleep 1
done
echo "Database is ready!"

# Run leader node tests
echo "Running leader node tests..."
go test -v ./nodes/leader/...

# Run leader node tests with race detector
echo "Running leader node tests with race detector..."
go test -race -v ./nodes/leader/...

# Cleanup
echo "Cleaning up..."
docker compose -f docker-compose.test.yml down -v
