#!/bin/bash

FILE="static-key/leadernode.bin"

echo "Checking for existing leader node file: $FILE"

# Check if the file exists
if [ -f "$FILE" ]; then
    echo "File '$FILE' found. Skipping peer ID generation."
    
    # Run Docker Compose with the existing file
    echo "Running docker compose up with --build..."
    docker compose up -d --build
else
    echo "File '$FILE' not found. Please generate new ID using run_generator.sh and update LEADER_PEER_ID in .env."
fi
