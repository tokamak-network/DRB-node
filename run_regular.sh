#!/bin/bash

# Set environment variables for the regular node (remaining in .env file)
export NODE_TYPE="regular"

# Run the regular node with the leader node's private key in the background
go run cmd/main.go &
