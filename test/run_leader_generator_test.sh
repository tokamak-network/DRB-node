#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Change to test directory so static-key is created in test/static-key
cd "$SCRIPT_DIR"

GO_FILE_PATH="../cmd/generator/main.go"

BINARY_NAME="peer_id_generator"

TARGET_FILE="static-key/leadernode.bin"

echo "Generating $TARGET_FILE from $GO_FILE_PATH..."


if [ -f "$TARGET_FILE" ]; then
    echo "File '$TARGET_FILE' already exists. The script will not generate a new peer ID."
else

    echo "Building the Go program..."
    go build -o "$BINARY_NAME" "$GO_FILE_PATH"


    if [ $? -eq 0 ]; then
        echo "Build successful. Executing the program..."
       
        chmod +x "$BINARY_NAME"
        ./"$BINARY_NAME"
    else
        echo "Go build failed. Exiting."
        exit 1
    fi
fi
