#!/bin/bash

# Change to project root directory
cd "$(dirname "$0")/.." || exit 1

# Start the test database
echo "Starting test database..."
docker compose -f test/docker-compose.test.yml up -d

# Wait for database to be ready
echo "Waiting for database to be ready..."
until docker exec testdb pg_isready -U postgres > /dev/null 2>&1; do
    echo -n "."
    sleep 1
done
echo "Database is ready!"

# Run utils tests
echo "Running utils tests..."
go test -v ./utils/...

# Cleanup
echo "Cleaning up..."
docker compose -f test/docker-compose.test.yml down -v
