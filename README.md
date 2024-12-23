# DRB Node Configuration Guide

This document provides comprehensive instructions for configuring and running a **Distributed Random Beacon (DRB)** node. The DRB node operates in two modes: **Leader Node** and **Regular Node**. Follow the setup steps according to the node type you intend to run.

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

```
# Leader Node Configuration
PEER_ID=leadernode
LEADER_PORT=61280
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
NODE_TYPE=leader

#### Shared Configurations
ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>
Regular Node Configuration


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
Running the Node
You can run the DRB Node using one of the following methods:

1. Direct Execution
Run the node directly with Go:

go run cmd/main.go
2. Build and Execute
Generate a binary file and execute it:

Build the node:

go build -o drb-node cmd/main.go
Run the executable:

./drb-node
3. Using Docker
Build and run the node in a containerized environment:

Ensure Docker is installed, and the .env file is correctly configured.

docker-compose up --build
Repository Structure
The repository is organized into several directories based on functionality. Here is a breakdown of the main folders and files:


├── cmd/                          # Entry point for running the DRB Node
│   └── main.go                    # Main file to start the DRB node
├── contracts/                     # Folder containing contract ABI files
│   └── commit2reveal_abi.json    # ABI file for the Commit2RevealDRB smart contract
├── eth/                           # Ethereum-related functions for smart contract interactions
│   └── eth.go                    # Ethereum client functions and smart contract interaction
├── libp2putils/                   # Helper utilities for libp2p peer-to-peer communication
│   └── libp2putils.go            # Libp2p utilities for handling peer-to-peer communication
├── nodes/                         # Core functions for managing nodes, including registration and communication
│   ├── leaderNode.go             # Logic for the Leader Node (managing commitments, Merkle root generation)
│   ├── regularNode.go            # Logic for the Regular Node (commitment submission, deposit check)
│   ├── leaderNode_helper/        # Helper functions for Leader Node
│   │   ├── acceptSecretValue.go  # Helper function for handling secret value submission
│   │   ├── registerNode.go      # Helper function for node registration
│   │   └── monitorCommits.go    # Helper function for monitoring commitments from regular nodes
│   └── regularNode_helper/       # Helper functions for Regular Node
│       ├── generateCvsSignature.go # Helper function for generating CVS signatures
│       └── handleCommitRequest.go # Helper function for handling commitment requests from the leader
├── commit-reveal2/                # Logic for generating commitments, Merkle tree, and reveal order
│   ├── commit.go                 # Logic for commitment generation and Merkle tree handling
│   ├── merkleTree.go             # Logic for Merkle tree root generation
│   ├── reveal_order.go           # Logic for determining the reveal order for committed nodes
├── transactions/                  # Functions to handle Ethereum transactions
│   ├── callFunction.go           # Smart contract interaction (helper function for calling contract methods)
│   ├── execute.go                # Helper function for executing Ethereum transactions
├── utils/                         # Utility functions for various tasks (e.g., signing, IP retrieval)
│   ├── clients.go                # Ethereum client setup and contract ABI loading
│   ├── commit.go                 # Commit data structures and commit data management
│   ├── graphql_queries.go        # GraphQL queries for fetching round data and activated operators
│   ├── ip_retriever.go           # Retrieves local and public IP addresses
│   ├── leaderNodeData.go         # Logic for handling leader commit data
│   ├── node_info.go              # Logic for saving/loading node information
│   ├── peer_id_storage.go        # Handles storing and loading libp2p PeerID
│   ├── streamHandler.go          # Handles libp2p stream creation and data sending
│   ├── utils.go                  # Various utility functions (e.g., signature verification)
├── .env                           # Configuration file for environment variables
├── README.md                      # This file
nodes/ Folder
The nodes/ folder contains the core logic for managing node operations, including registration, activation, communication, and interaction between leader and regular nodes.

leaderNode.go: Implements the behavior of the Leader Node, including the registration of nodes, processing of commitments, generating Merkle roots, and submitting data to Ethereum.
regularNode.go: Implements the behavior of the Regular Node, handling peer-to-peer communication, deposit checks, and commitment submissions to the Leader Node.
leaderNode_helper/: Contains helper functions that assist with leader node operations such as registration, commitment monitoring, and handling secret values.
regularNode_helper/: Contains helper functions specific to the regular node, such as generating CVS signatures and handling commit requests from the leader node.
Core Functions
Below are the core functions and their responsibilities across different components:

Leader Node (leaderNode.go)
RunLeaderNode: Initializes the Leader Node, processes incoming commit data, and generates Merkle roots when all commitments are received.
handleCommitRequest: Processes CVS commit data from regular nodes.
generateMerkleRoot: Generates the Merkle Root from the collected CVS values.
submitMerkleRoot: Submits the Merkle Root to Ethereum.
Regular Node (regularNode.go)
RunRegularNode: Initializes the Regular Node, checks deposits, and sends commitments to the Leader Node.
sendCommitToLeader: Sends the generated commit (CVS) to the Leader Node.
sendCosToLeader: Sends the Commitment Output (COS) to the Leader Node after verification.
Verifying the Setup
After running the node, you can verify the setup using the following methods:

Logs:
Check the logs to confirm successful peer connections. Look for entries indicating successful connections and Ethereum transactions.

Regular Node Connections:
Ensure that the Regular Nodes are connected to the Leader Node. You can check this in the logs or by inspecting the peer connections.

On-Chain Interactions:
Use your Ethereum RPC provider to monitor and verify on-chain interactions, such as random number generation and Merkle root submissions. You can check the contract for updates using a tool like Etherscan or any Ethereum block explorer.

Contributing to the Project
To contribute to the project, follow these steps:

Fork the repository: Create a personal fork of the repository.
Clone the repository: Clone your fork to your local machine.

git clone https://github.com/tokamak-network/DRB-node
branch: main
Make your changes: Modify or add new features as needed.
Test the changes: Ensure your changes do not break the existing functionality by running tests and verifying the setup.
Submit a Pull Request: Once your changes are ready, submit a pull request with a description of your changes.