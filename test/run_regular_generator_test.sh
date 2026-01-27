#!/bin/bash


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Change to test directory so static-key is created in test/static-key
cd "$SCRIPT_DIR"

GO_FILE_PATH="../cmd/regulargenerator/main.go"

BINARY_NAME="regular_peer_id_generator"

TARGET_FILE_1="static-key/regularnode1.bin"
TARGET_FILE_2="static-key/regularnode2.bin"
TARGET_FILE_3="static-key/regularnode3.bin"

echo "Generating regular node peer IDs from $GO_FILE_PATH..."


FILES_EXIST=false
if [ -f "$TARGET_FILE_1" ]; then
    echo "File '$TARGET_FILE_1' already exists."
    FILES_EXIST=true
fi
if [ -f "$TARGET_FILE_2" ]; then
    echo "File '$TARGET_FILE_2' already exists."
    FILES_EXIST=true
fi
if [ -f "$TARGET_FILE_3" ]; then
    echo "File '$TARGET_FILE_3' already exists."
    FILES_EXIST=true
fi

if [ "$FILES_EXIST" = true ]; then
    echo "Some regular node key files already exist. The script will load existing peer IDs or generate missing ones."
    echo "To generate a new peer ID, please delete the existing file first."
fi


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

