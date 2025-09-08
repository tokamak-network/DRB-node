#!/bin/bash

# Define the file path for the Go source
GO_FILE_PATH="cmd/run-generator/main.go"
# Define the name for the compiled binary
BINARY_NAME="peer_id_generator"
# Define the target file to be checked/created
TARGET_FILE="static-key/leadernode.bin"

echo "Generating $TARGET_FILE from $GO_FILE_PATH..."

# Check if the target file already exists
if [ -f "$TARGET_FILE" ]; then
    echo "File '$TARGET_FILE' already exists. The script will not generate a new peer ID."
else
    # Build the Go program
    echo "Building the Go program..."
    go build -o "$BINARY_NAME" "$GO_FILE_PATH"

    # Check if the build was successful
    if [ $? -eq 0 ]; then
        echo "Build successful. Executing the program..."
        # Run the compiled program
        chmod +x "$BINARY_NAME"
        ./"$BINARY_NAME"
        echo "Please update the LEADER_PEER_ID in .env."
    else
        echo "Go build failed. Exiting."
        exit 1
    fi
fi
