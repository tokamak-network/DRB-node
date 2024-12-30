#!/bin/bash

# Set environment variables for the leader node (remaining in .env file)
export NODE_TYPE="leader"

# Run the leader node in the background
go run cmd/main.go &
