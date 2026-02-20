#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

LEADER_FILE="$SCRIPT_DIR/static-key/leadernode.bin"
REGULAR_FILE_1="$SCRIPT_DIR/static-key/regularnode1.bin"
REGULAR_FILE_2="$SCRIPT_DIR/static-key/regularnode2.bin"
REGULAR_FILE_3="$SCRIPT_DIR/static-key/regularnode3.bin"

echo "Checking for required key files..."

MISSING_FILES=()

# Check if leader node file exists
if [ ! -f "$LEADER_FILE" ]; then
    MISSING_FILES+=("$LEADER_FILE")
fi

# Check if regular node files exist
if [ ! -f "$REGULAR_FILE_1" ]; then
    MISSING_FILES+=("$REGULAR_FILE_1")
fi
if [ ! -f "$REGULAR_FILE_2" ]; then
    MISSING_FILES+=("$REGULAR_FILE_2")
fi
if [ ! -f "$REGULAR_FILE_3" ]; then
    MISSING_FILES+=("$REGULAR_FILE_3")
fi

# If any files are missing, print error and exit
if [ ${#MISSING_FILES[@]} -gt 0 ]; then
    echo "Error: The following key files are missing:"
    for file in "${MISSING_FILES[@]}"; do
        echo "  - $file"
    done
    echo ""
    echo "Please generate the missing peer IDs:"
    if [ ! -f "$LEADER_FILE" ]; then
        echo "  - Leader node: ./test/run_leader_generator_test.sh"
    fi
    if [ ! -f "$REGULAR_FILE_1" ] || [ ! -f "$REGULAR_FILE_2" ] || [ ! -f "$REGULAR_FILE_3" ]; then
        echo "  - Regular nodes: ./test/run_regular_generator_test.sh"
    fi
    exit 1
fi

echo "All required key files found. Building and starting services..."
echo ""

# Verify .env file exists in project root
if [ ! -f "$PROJECT_ROOT/.env" ]; then
    echo "Error: .env file not found in project root ($PROJECT_ROOT/.env)"
    exit 1
fi

# Change to test directory
cd "$SCRIPT_DIR"

echo "Cleaning up existing containers..."
docker compose --env-file ../.env down 2>/dev/null || true



echo "Building and starting all test nodes (1 leader + 3 regular nodes)..."
docker compose --env-file ../.env up -d --build

if [ $? -eq 0 ]; then
    echo ""
    echo "All services built and started successfully!"
else
    echo ""
    echo "Failed to build/start services"
    exit 1
fi

