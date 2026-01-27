#!/bin/bash

# Script to run Geth test node in a separate process
# This starts Geth, deploys contracts, and keeps it running until interrupted

set -e

echo "🚀 Starting Geth test node..."
echo "📝 This will start Geth in dev mode, deploy contracts, and keep it running"
echo "⏹️  Press Ctrl+C to stop"
echo ""

# Create temporary data directory
tmpDir=$(mktemp -d -t geth-test-XXXXXX)
if [ $? -ne 0 ] || [ -z "$tmpDir" ]; then
    echo "❌ Failed to create temp dir" >&2
    exit 1
fi

# Cleanup function to remove temp directory
cleanup() {
    if [ -n "$tmpDir" ] && [ -d "$tmpDir" ]; then
        echo "🧹 Cleaning up temporary directory..."
        rm -rf "$tmpDir"
    fi
}
trap cleanup EXIT INT TERM

# Run geth with the specified flags
geth \
    --dev \
    --dev.period 1 \
    --datadir "$tmpDir" \
    --http \
    --http.api eth,net,web3,debug,personal,admin \
    --http.addr 0.0.0.0 \
    --http.port 8545 \
    --http.corsdomain "*" \
    --http.vhosts "*" \
    --ws \
    --ws.api eth,net,web3,debug,personal,admin \
    --ws.addr 0.0.0.0 \
    --ws.port 8546 \
    --ws.origins "*" \
    --allow-insecure-unlock \
    --nodiscover \
    --maxpeers 0 \
    --miner.gasprice 1000000000 \
    --verbosity 3

