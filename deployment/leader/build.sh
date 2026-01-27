#!/bin/bash

# Build and start leader node
echo "Building and starting Leader Node..."
echo ""

# Change to leader deployment directory
cd "$(dirname "$0")"

PROJECT_ROOT="$(cd ../.. && pwd)"


if [ ! -f "$PROJECT_ROOT/.env" ]; then
    echo "Error: .env file not found in project root ($PROJECT_ROOT/.env)"
    exit 1
fi

docker compose --env-file ../../.env up -d --build

# Check if build was successful
if [ $? -eq 0 ]; then
    echo ""
    echo "Leader Node built and started successfully!"
else
    echo ""
    echo "Failed to build/start Leader Node"
    exit 1
fi

