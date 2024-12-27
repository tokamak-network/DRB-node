#!/bin/bash

# Set environment variables for the leader node
export LEADER_PRIVATE_KEY=""
export LEADER_EOA=""
export LEADER_PORT=61280
export NODE_TYPE="leader"

export ETH_RPC_URL=""
export CONTRACT_ADDRESS=""
export SUBGRAPH_URL=""

# Run the leader node in the background
go run cmd/main.go &
