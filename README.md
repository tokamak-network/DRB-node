# DRB Node Configuration Guide

This document provides comprehensive instructions for configuring and running a **Distributed Random Beacon (DRB)** node. The DRB node operates in two modes: **Leader Node** and **Regular Node**. Follow the setup steps according to the node type you intend to run.

---

## Prerequisites

Before setting up the DRB node, ensure the following requirements are met:

1. **Install Go**:  
   Ensure Go is installed on your system. **Go version 1.23.3 or greater** is required. [Refer to the Go installation guide](https://go.dev/doc/install) for details.

2. **Install Docker**:  
   Docker is required for running the nodes. [Refer to Docker installation guide](https://docs.docker.com/get-docker/) for details.

3. **Smart Contract Deployment**:  
   Deploy the DRB smart contract and obtain its address.  
   You can get the DRB smart contract from [here](https://github.com/tokamak-network/Commit-Reveal2/tree/service).

4. **Account Balance**:  
   Ensure the Leader Node and Regular Node accounts have sufficient balance to perform transactions.
   - The **Leader Node** must have enough tokens to interact with the blockchain network, such as submitting Merkle roots and generating random numbers.
   - The **Regular Node** must have enough tokens to cover the deposit requirements set by the contract.

---

## Environment Variables

The `.env` file is required for node configuration. Below are the settings for each type of node:

### Leader Node Configuration

```bash
# Leader Node Configuration
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
ETH_RPC_URLS=<Your Ethereum RPC URLs separated by , (comma)>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
POSTGRES_PASSWORD=password
```

### Regular Node Configuration

```bash
# Regular Node Configuration
LEADER_PEER_ID=<Leader Node Peer ID>
LEADER_EOA=<Leader Ethereum Address>
EOA_PRIVATE_KEY_1=<Your Regular Node Private Key for Account 1>
EOA_PRIVATE_KEY_2=<Your Regular Node Private Key for Account 2>
EOA_PRIVATE_KEY_3=<Your Regular Node Private Key for Account 3>
CHAIN_ID=111551119090
POSTGRES_PASSWORD=password

ETH_RPC_URLS=<Your Ethereum RPC URLs separated by , (comma)>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
```

### Running the Node

## 1. Deploy the Smart Contract and Set Up Graph Node

Before running the DRB Node, follow these steps:

Deploy the Smart Contract:

Clone the repository for the DRB smart contract.
Deploy the contract to your preferred Ethereum network and obtain the contract address. The leader node should be owner of the contract, so deploy the contract with leader node private key.

## 2. Run the Nodes

After deploying the smart contract, you can proceed to run the nodes.

- **Step-by-Step Node Execution**
  Run the following command:
  `./build.sh`

The above command will run the leader node and three regular nodes.

If there is no leadernode.bin file, it will fail and display a message like

```
File static-key/leadernode.bin not found. Please generate new ID using run_generator.sh and update LEADER_PEER_ID in .env.
```

If you see this message, please generate leadernode.bin file using the command:
`./run_generator.sh`

If leadernode id generation succeeds, update `LEADER_PEER_ID` in `.env` and execute `./build.sh` to run the nodes in docker environment at once.

- If you have permission issue to above scripts, please run this command:

  ```
  chmod +x <filename>
  ```

- **Once the build is complete**: You can see the leader node logs via docker logs -f leadernode,
  docker logs -f regularnode1, docker logs -f regularnode2, docker logs -f regularnode3 are used to output the logs from the regular node

---

### Verifying the Setup

After running the node, you can verify the setup using the following methods:

#### **1. Node Registration Logs**

Check the logs to confirm successful peer connections. Look for entries indicating successful connections and Ethereum transactions.

- **For Regular Node Registration**, the logs should show like:

- **Success Case**:

"Registration request sent to leader"

- **Failure Case**:

"Failed to send registration request: <error>"

- **For Leader Node Registration**, after storing or updating data in the Leader Node file, the console should display a message:

- **Success Case**:

"Successfully registered or updated EOA 0x1123123123123123123123 with NodeInfo: IP=203.0.113.45, Port=30303, PeerID=16Uiu2HAmWY8f56cVGe6n6iV6Xg75GV7WqvG9zmNwe1t8H1JqV2fb"

- **Failure Case**:

"failed to save registered nodes: <error>"

"failed to activate EOA 0xdD4793AC06D46939078767E3C5ee04802aA4503b on-chain: <error>"

- After on-chain activation, the log should confirm that the node registration and activation were completed with a message:

"Activation successful"

- **Regular Node Connections**
  Ensure that the Regular Nodes are connected to the Leader Node. You can check this in the logs or by inspecting the peer connections.

- **On-Chain Interactions**
  Use your Ethereum RPC provider to monitor and verify on-chain interactions, such as random number generation and Merkle root submissions. You can check the contract for updates using a tool like Etherscan or any Ethereum block explorer.

log message for executing merkle root onchain (leader node):

- **Success Case**:

"Successfully submitted Merkle root for round 0 with trail 0"

"Transaction <hash> confirmed in block <block number>"

- **Failure Case**:

"Failed to submit Merkle root for round 0 with trail 0"

log message for execuitng generate random number onchain (leader node):

- **Success Case**:

"All EOAs have submitted for round 0 with trail 0. Initiating random number generation."

"Transaction submitted. TX Hash: <hash>"

- **Failure Case**:

"Failed to execute random number generation transaction for round 0 with trail 0"

---

### Repository Structure

The repository is organized into several directories based on functionality. Here is a breakdown of the main folders and files:

```
├── cmd/                          # Entry point for running the DRB Node
│   └── main.go                   # Main file to start the DRB node
├── commit-reveal2/               # Logic for generating commitments, Merkle tree, and reveal order
│   ├── commit.go                 # Logic for commitment generation and Merkle tree handling
│   ├── merkleTree.go             # Logic for Merkle tree root generation
│   └── reveal_order.go           # Logic for determining the reveal order for committed nodes
├── contract/                     # Smart contract related files
│   ├── abi/                      # Contract ABI files
│   │   ├── Commit2RevealDRB.abi  # Contract ABI file
│   │   ├── Commit2RevealDRB.bin  # Contract bytecode
│   │   └── Commit2RevealDRB.json # Contract metadata and ABI
│   └── Commit2RevealDRB/         # Generated Go bindings
│       ├── Commit2RevealDRB.go   # Go contract bindings
│       └── Commit2RevealDRB.sol  # Solidity contract source
├── database/                     # Database layer for persistent storage
│   ├── migrations/               # Database migration files
│   │   └── 001_init_schema.sql   # Initial database schema
│   ├── batch_delete.go           # Batch deletion operations
│   ├── broadcast_tracker.go      # Tracking broadcast messages
│   ├── config.go                 # Database configuration
│   ├── connection.go             # Database connection management
│   ├── leader_commit.go          # Leader commit data operations
│   ├── migrations.go             # Migration execution logic
│   ├── node_info.go              # Node information storage
│   ├── peer_commit.go            # Peer commit data operations
│   ├── regular_commit.go         # Regular node commit operations
│   ├── reveal_order.go           # Reveal order storage
│   └── scheme.go                 # Database schema definitions
├── eth/                          # Ethereum client and smart contract interactions
│   ├── eth.go                    # Core Ethereum client functions and transaction handling
│   └── eth_test.go               # Tests for Ethereum functionality
├── libp2putils/                  # Helper utilities for libp2p peer-to-peer communication
│   └── libp2putils.go           # Libp2p utilities for handling peer-to-peer communication
├── logger/                       # Logging utilities
│   └── logger.go                 # Centralized logging configuration
├── nodes/                        # Core node implementation and logic
│   ├── leaderNode.go             # Leader Node implementation (registration, commitment processing)
│   ├── regularNode.go            # Regular Node implementation (deposit, activation, commitment submission)
│   ├── leaderNode_helper/        # Helper functions for Leader Node operations
│   │   ├── acceptCommit.go       # Event processing and monitoring logic
│   │   ├── broadCast.go          # Broadcasting messages to regular nodes
│   │   ├── monitor_commits.go    # Monitoring commitments and round completion
│   │   ├── registration_helper.go # Node registration handling
│   │   ├── reveal_requests.go    # Secret value request management and monitoring
│   │   └── secret_value_handler.go # Secret value processing and verification
│   └── regularNode_helper/       # Helper functions for Regular Node operations
│       ├── generateCvsSignature.go # CVS signature generation
│       ├── receiveValues.go      # Processing received values from leader
│       ├── secretValueHandler.go # Handling secret value requests
│       └── sendCommit.go         # Commitment submission and event monitoring
├── pkg/                          # Reusable packages and utilities
│   ├── constants/                # Project constants
│   │   └── chain.go              # Chain-specific constants
│   ├── fallback_ethclient/       # Fallback Ethereum client for reliability
│   │   └── fallback_rpc.go       # Multi-RPC client with failover support
│   └── types/                    # Common type definitions
│       └── networkinfo.go        # Network information structures
├── static-key/                   # Static key storage for leader node
│   └── leadernode.bin           # Leader node key file
├── utils/                        # Utility functions and helpers
│   ├── broadcast.go              # Broadcasting utilities and message structures
│   ├── clients.go                # Ethereum client setup and contract ABI loading
│   ├── commit.go                 # Commit data structures and management
│   ├── ip_retriever.go           # IP address retrieval utilities
│   ├── node_info.go              # Node information handling
│   ├── reveal_order.go           # Reveal order utilities
│   ├── streamHandler.go          # LibP2P stream handling
│   └── utils.go                  # General utility functions (signature verification, etc.)
├── docker-compose.yml            # Docker Compose configuration for multi-node setup
├── Dockerfile                    # Docker container configuration
├── go.mod                        # Go module dependencies
├── go.sum                        # Go module checksums
├── main                          # Compiled binary
└── README.md                     # This documentation file
```

### nodes/ Folder

The `nodes/` folder contains the core logic for managing node operations, including registration, activation, communication, and interaction between leader and regular nodes.

- **`leaderNode.go`**: Implements the behavior of the Leader Node, including node registration, processing of commitments, generating Merkle roots, and coordinating the DRB protocol across multiple rounds.
- **`regularNode.go`**: Implements the behavior of the Regular Node, handling peer-to-peer communication, deposit/activation management, and commitment submissions to the Leader Node with monitoring capabilities.

#### leaderNode_helper/ Directory

Contains specialized helper functions for Leader Node operations:

- **`acceptCommit.go`**: Core event processing and monitoring logic, including Status events, CVS/COS submissions, secret submissions, and time-based monitoring for automatic requests and failure handling.
- **`broadCast.go`**: Broadcasting messages to regular nodes with acknowledgment tracking and retry mechanisms, including synchronous broadcasting for critical operations.
- **`monitor_commits.go`**: Monitors commitments and round completion, validates blockchain state, and triggers random number generation when all requirements are met.
- **`registration_helper.go`**: Handles node registration, EOA activation verification, and maintains the registry of participating nodes.
- **`reveal_requests.go`**: Manages the sequential secret value request process, including timing-based monitoring for submission failures and automatic retry mechanisms.
- **`secret_value_handler.go`**: Processes and verifies secret values from regular nodes, implements sequential revelation protocol, and manages hash verification.

#### regularNode_helper/ Directory

Contains specialized helper functions for Regular Node operations:

- **`generateCvsSignature.go`**: Generates CVS (Commitment Value Signature) for commitment verification and handles signature creation for various protocol messages.
- **`receiveValues.go`**: Processes received values from the leader node, handles event subscriptions, and manages acknowledgment responses with signature verification.
- **`secretValueHandler.go`**: Handles secret value requests from the leader, implements sequential revelation checks, and ensures proper order verification before secret submission.
- **`sendCommit.go`**: Manages commitment submission to the leader and implements comprehensive event monitoring with time-based failure detection and automatic retry mechanisms.

### Key Components and Architecture

The DRB node system is built around several key architectural components:

#### Database Layer (`database/`)

- **Persistent Storage**: PostgreSQL-based storage for commit data, node information, and reveal orders
- **Migration System**: Automatic database schema management and updates
- **Data Models**: Structured storage for leader commits, regular commits, broadcast tracking, and node registry

#### Network Layer (`libp2putils/`, `pkg/fallback_ethclient/`)

- **P2P Communication**: LibP2P-based peer-to-peer networking for node communication
- **Blockchain Connectivity**: Fallback RPC client with automatic failover for reliable Ethereum connectivity
- **Event Monitoring**: Real-time blockchain event subscription and processing

#### Protocol Implementation (`commit-reveal2/`, `nodes/`)

- **Commit-Reveal Protocol**: Multi-phase cryptographic commitment and revelation process
- **Merkle Tree Generation**: Efficient proof generation for commitment verification
- **Time-Based Monitoring**: Automatic request submission and failure detection with configurable timeouts
- **Sequential Revelation**: Ordered secret revelation process with verification

#### Monitoring and Reliability

- **Event-Driven Architecture**: Responsive to blockchain events with automatic state transitions
- **Failure Detection**: Time-based monitoring for missing submissions with automatic retry
- **State Validation**: Blockchain state verification to prevent stale operations
- **Graceful Degradation**: Robust error handling and recovery mechanisms

### Core Protocol Flow

1. **Node Registration**: Regular nodes register with the leader and activate on-chain
2. **Round Initialization**: Status events trigger new DRB rounds with automatic monitoring
3. **Commitment Phase**: Regular nodes submit CVS values with time-based request automation
4. **Revelation Phase**: Sequential merkle root submission followed by COS submission
5. **Secret Phase**: Ordered secret revelation with broadcast verification
6. **Completion**: Random number generation and round finalization

### Contributing to the Project

To contribute to the project, follow these steps:

1. **Fork the repository**: Create a personal fork of the repository.
2. **Clone the repository**: Clone your fork to your local machine.

`git clone https://github.com/tokamak-network/DRB-node`

3. **branch**: dispute-mechanishm
4. **Make your changes**: Modify or add new features as needed.
5. **Submit a Pull Request**: Once your changes are ready, submit a pull request with a description of your changes.

### **Bugs/Error s**

**Observed Issue:**

- When the leader node receives a high volume of commit/reveal values from regular nodes simultaneously, it sometimes fails to store one or more values due to file I/O contention or other concurrency issues.
