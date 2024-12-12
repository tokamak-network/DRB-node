# DRB Node Configuration Guide

This document provides comprehensive instructions for configuring and running a Distributed Random Beacon (DRB) node. The DRB node operates in two modes: **Leader Node** and **Regular Node**. Follow the setup steps according to the node type you intend to run.

---

## Prerequisites

Before setting up the DRB node, ensure the following requirements are met:

1. **Install Go**:  
   Ensure Go is installed on your system. [Refer to the Go installation guide](https://go.dev/doc/install) for details.

2. **Install Docker (optional)**:  
   Docker is required if you prefer running the node in a containerized environment.

3. **Smart Contract Deployment**:  
   Deploy the DRB smart contract and obtain its address.

4. **Graph Node**:  
   A running Subgraph instance is required for monitoring on-chain events.

---

## Environment Variables

The `.env` file is required for node configuration. Below are the settings for each type of node:

### Leader Node Configuration

# Leader Node Configuration
PEER_ID=leadernode
LEADER_PORT=61280
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
NODE_TYPE=leader

# Shared Configurations
ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>

# Regular Node Configuration
LEADER_IP=<Leader Node IP Address>
LEADER_PORT=61280
LEADER_PEER_ID=<Leader Node Peer ID>
LEADER_EOA=<Leader Ethereum Address>
PEER_ID=regularNode
EOA_PRIVATE_KEY=<Your Regular Node Private Key>
NODE_TYPE=regular
PORT=61281
CHAIN_ID=111551119090

# Shared Configurations
ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>

# Regular Node Configuration
LEADER_IP=<Leader Node IP Address>
LEADER_PORT=61280
LEADER_PEER_ID=<Leader Node Peer ID>
LEADER_EOA=<Leader Ethereum Address>
PEER_ID=regularNode
EOA_PRIVATE_KEY=<Your Regular Node Private Key>
NODE_TYPE=regular
PORT=61281
CHAIN_ID=111551119090

# Shared Configurations
ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>

# Running the Node
You can run the DRB Node using one of the following methods:

# 1. Direct Execution
Run the node directly with Go:

go run cmd/main.go

# 2. Build and Execute
Generate a binary file and execute it:

Build the node:
go build -o drb-node cmd/main.go

Run the executable:
./drb-node

# 3. Using Docker
Build and run the node in a containerized environment:

Ensure Docker is installed, and the .env file is correctly configured.

``` Run the following command: ```

docker-compose up --build

Verifying the Setup
Logs:
Check the logs to confirm successful peer connections.

Regular Node Connections:
Verify that Regular Nodes are connected to the Leader Node.

On-Chain Interactions:
Use your Ethereum RPC provider to monitor and verify on-chain interactions.

