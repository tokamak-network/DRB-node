#!/bin/bash

# Set environment variables for the regular node
export NODE_TYPE="regular"
export EOA_PRIVATE_KEY=""
export LEADER_PEER_ID=""
export LEADER_IP=""
export PORT=61281
export CHAIN_ID=
export PEER_ID="regularNode"
export LEADER_PORT=
export LEADER_EOA=""

export ETH_RPC_URL=""
export CONTRACT_ADDRESS=""
export SUBGRAPH_URL=""

# Run the regular node with the leader node's private key in the background
go run cmd/main.go &
