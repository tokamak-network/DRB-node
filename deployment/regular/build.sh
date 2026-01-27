#!/bin/bash

# Build and start regular node
echo "Building and starting Regular Node..."
echo ""

# Change to regular deployment directory
cd "$(dirname "$0")"

# Get project root 
PROJECT_ROOT="$(cd ../.. && pwd)"

# Verify .env file exists
if [ ! -f "$PROJECT_ROOT/.env" ]; then
    echo "Error: .env file not found in project root ($PROJECT_ROOT/.env)"
    exit 1
fi


docker compose --env-file ../../.env up -d --build

# Check if build was successful
if [ $? -eq 0 ]; then
    echo ""
    echo "Regular Node built and started successfully!"
else
    echo ""
    echo "Failed to build/start Regular Node"
    exit 1
fi
