# DRB Node Configuration Guide

This document provides comprehensive instructions for configuring and running a **Distributed Random Beacon (DRB)** node. The DRB node operates in two modes: **Leader Node** and **Regular Node**. Follow the setup steps according to the node type you intend to run.

---

## Prerequisites

Before setting up the DRB node, ensure the following requirements are met:

1. **Install Go**:  
   Ensure Go is installed on your system. **Go version 1.23.3 or greater** is required. [Refer to the Go installation guide](https://go.dev/doc/install) for details.

2. **Install Docker (optional)**:  
   Docker is required if you prefer running the node in a containerized environment. [Refer to Docker installation guide](https://docs.docker.com/get-docker/) for details.

3. **Smart Contract Deployment**:  
   Deploy the DRB smart contract and obtain its address.  
   You can get the DRB smart contract from [here](https://github.com/tokamak-network/Commit-Reveal-DRB/tree/commit-reveal-with-unpredictability-titan-sepolia).

4. **Graph Node**:  
   A running Subgraph instance is required for monitoring on-chain events.  
   The Subgraph repository is available [here](https://github.com/tokamak-network/DRB-subgraph).

5. **Ethereum Account Balance**:  
   Ensure the Leader Node and Regular Node accounts have sufficient balance to perform transactions.  
   - The **Leader Node** must have enough ETH to interact with the Ethereum network, such as submitting Merkle roots and generating random numbers.
   - The **Regular Node** must have enough ETH to cover the deposit requirements set by the contract.

---

## Environment Variables

The `.env` file is required for node configuration. Below are the settings for each type of node:

### Leader Node Configuration

```bash
# Leader Node Configuration
PEER_ID=leadernode
LEADER_PORT=61280
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
NODE_TYPE=leader

ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>
```

### Regular Node Configuration
```bash
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

ETH_RPC_URL=<Your Ethereum RPC URL>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
SUBGRAPH_URL=<Your Subgraph URL>
```

### Running the Node

## 1. Deploy the Smart Contract and Set Up Graph Node
Before running the DRB Node, follow these steps:

Deploy the Smart Contract:

Clone the repository for the DRB smart contract.
Deploy the contract to your preferred Ethereum network and obtain the contract address.
Run the Graph Node and Deploy Subgraph:

Clone the DRB Subgraph repository.
Deploy the Subgraph and ensure it is running to track on-chain events for your smart contract.

## 2. Run the Nodes
After deploying the smart contract and running the Subgraph, you can proceed to run the nodes.

- **Step-by-Step Node Execution**
Note: Always run the Leader Node first, and then start at least 2 Regular Nodes for proper network setup.

- **1. Leader Node**
To run the Leader Node individually, use the following script:

Set the environment variables first in leader bash file.

```bash
./run_leader.sh
```

- **2. Regular Nodes**
After the Leader Node is running, you can run the Regular Node with the following script separately:

Set the environment variables first in regular bash file.

```bash
./run_regular.sh
```

The Regular Node will use the Leader's private key as required. You can also start multiple Regular Nodes by running the script multiple times.

## 3. Other Ways to Run Node
You can run the DRB Node using one of the following methods:

- **1. Using the Combined Start Script**
If you prefer to run both Leader and Regular Nodes in one go, use the combined start script:

Set the environment variables first in leader and regular bash file to avoid any conflicts.

```bash
./start_drb_nodes.sh
```

- **2. Run Directly**

```bash
go run cmd/main.go --nodeType leader
```

- **3. Build and Execute**
Generate a binary file and execute it:

Build the node:
```bash
go build -o drb-node cmd/main.go
./drb-node
```

- **4. Using Docker**
Build and run the node in a containerized environment:

Ensure Docker is installed, and the .env file is correctly configured.
```bash
docker-compose up --build
```

## 3. Stopping the Nodes
To stop the nodes, use the following script:

```bash
./stop_drb_nodes.sh
```

This will stop any processes running on the specified ports.


### Troubleshooting Tips
Here are some common issues you might encounter and their solutions:

- **Issue**: Unable to connect to Ethereum RPC.
- **Solution**: Check if your Ethereum RPC URL is correctly configured in the .env file. Ensure that the Ethereum node is running and accessible.

- **Issue**: Node not connecting to Leader Node.
- **Solution**: Ensure that the IP, port, and Peer ID of the Leader Node are correctly set in the Regular Node's .env configuration. Check the service.log file for error messages related to peer connections.

- **Issue**: Insufficient balance for transaction.
- **Solution**: Ensure that the Leader Node and Regular Node Ethereum accounts have enough balance to perform transactions and make the required deposit.

--------------------------------------------------------------------------------------------------------

### Verifying the Setup

After running the node, you can verify the setup using the following methods:

#### **1. Logs**
Check the logs to confirm successful peer connections. Look for entries indicating successful connections and Ethereum transactions.

- **For Regular Node Registration**, the logs should show something like:
Registration request sent to leader.

- **For Leader Node Registration**, after storing or updating data in the Leader Node file, the console should display a message similar to:

Successfully registered or updated EOA 0x1123123123123123123123 with NodeInfo: IP=203.0.113.45, Port=30303, PeerID=16Uiu2HAmWY8f56cVGe6n6iV6Xg75GV7WqvG9zmNwe1t8H1JqV2fb.


- After on-chain activation, the log should confirm that the node registration and activation were completed with a message like:
Node registration and activation completed.

- **Regular Node Connections**
Ensure that the Regular Nodes are connected to the Leader Node. You can check this in the logs or by inspecting the peer connections.

- **On-Chain Interactions**
Use your Ethereum RPC provider to monitor and verify on-chain interactions, such as random number generation and Merkle root submissions. You can check the contract for updates using a tool like Etherscan or any Ethereum block explorer.

Example log message:
Successfully submitted Merkle root for round <round number>

- **Service Logs**
Check the service.log file for connection status, errors, and other critical messages related to the node operation.

Example log message:
Connected to regular node at <node IP> on port <port>

--------------------------------------------------------------------------------------------------------

### Repository Structure
The repository is organized into several directories based on functionality. Here is a breakdown of the main folders and files:

```
├── cmd/                          # Entry point for running the DRB Node
│   └── main.go                    # Main file to start the DRB node
├── contracts/                     # Folder containing contract ABI files
│   └── abi                         # ABI file for the Commit2RevealDRB smart contract
│       ├── Commit2RevealDRB.json
├── eth/                           # Ethereum-related functions for smart contract interactions
│   └── eth.go                    # Ethereum client functions and smart contract interaction
├── libp2putils/                   # Helper utilities for libp2p peer-to-peer communication
│   └── libp2putils.go            # Libp2p utilities for handling peer-to-peer communication
├── nodes/                         # Core functions for managing nodes, including registration and communication
│   ├── leaderNode.go             # Logic for the Leader Node (managing commitments, Merkle root generation)
│   ├── regularNode.go            # Logic for the Regular Node (commitment submission, deposit check)
│   ├── leaderNode_helper/        # Helper functions for Leader Node
│   │   ├── secret_value_handler.go  # Helper function for handling secret value submission
│   │   ├── registration_helper.go   # Helper function for node registration
│   │   ├── monitorCommits.go    # Helper function for monitoring commitments from regular nodes
│   │   └── reveal_requests.go   # Helper function for managing secret value requests from regular nodes
│   └── regularNode_helper/       # Helper functions for Regular Node
│       ├── generateCvsSignature.go # Helper function for generating CVS signatures
│       └── handleCommitRequest.go # Helper function for handling commitment requests from the leader node
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
```

### nodes/ Folder
The `nodes/` folder contains the core logic for managing node operations, including registration, activation, communication, and interaction between leader and regular nodes.

- **`leaderNode.go`**: Implements the behavior of the Leader Node, including the registration of nodes, processing of commitments, generating Merkle roots, and submitting data to Ethereum.
- **`regularNode.go`**: Implements the behavior of the Regular Node, handling peer-to-peer communication, deposit checks, and commitment submissions to the Leader Node.
- **`leaderNode_helper/`**: Contains helper functions for Leader Node operations such as registration, commitment monitoring, and handling secret values.
  - **`secret_value_handler.go`**: Handles the secret value submission from regular nodes.
  - **`registration_helper.go`**: Manages the registration of nodes.
  - **`monitorCommits.go`**: Monitors commitments, generates Merkle roots, and manages random number generation for rounds.
  - **`reveal_requests.go`**: Manages sending and receiving secret value requests from regular nodes.
- **`regularNode_helper/`**: Contains helper functions specific to the regular node, such as generating CVS signatures and handling commit requests from the leader node.

### Core Functions
Below are the core functions and their responsibilities across different components:

### Leader Node (leaderNode.go)

- **RunLeaderNode**: Initializes the Leader Node, processes incoming commit data, and generates Merkle roots when all commitments are received.
- **handleCommitRequest**: Processes CVS commit data from regular nodes.
- **generateMerkleRoot**: Generates the Merkle Root from the collected CVS values.
- **submitMerkleRoot**: Submits the Merkle Root to Ethereum.
- **monitorCommits**: Monitors the status of commitments from regular nodes, checks for completed rounds, and triggers necessary actions such as generating random numbers.

### Regular Node (regularNode.go)

- **RunRegularNode**: Initializes the Regular Node, checks deposits, and sends commitments to the Leader Node.
- **sendCommitToLeader**: Sends the generated commit (CVS) to the Leader Node.
- **sendCosToLeader**: Sends the Commitment Output (COS) to the Leader Node after verification.
- **generateRandomNumber**: Generates a random number after all commitments have been received and processed.
- **checkActivationStatus**: Checks if the Regular Node's Ethereum Address (EOA) is activated for a given round.

### Helper Functions for Leader Node (leaderNode_helper/)

- **StartSecretValueRequests**: Initiates the process of requesting secret values from regular nodes according to the reveal order.
- **sendSecretValueRequestToNode**: Sends the secret value request to a specific regular node, signing the round number and ensuring the correct node is targeted.
- **HandleSecretValueResponse**: Handles responses to secret value requests and continues sending requests to the next node in the reveal order.

### Helper Functions for Regular Node (regularNode_helper/)

- **generateCvsSignature**: Generates the CVS (Commitment Value Signature) for verifying the commitment.
- **handleCommitRequest**: Handles incoming commitment requests from the leader and processes the CVS (Commitment Value Signature).

### Contributing to the Project
To contribute to the project, follow these steps:


1. **Fork the repository**: Create a personal fork of the repository.
2. **Clone the repository**: Clone your fork to your local machine.

``` git clone https://github.com/tokamak-network/DRB-node ```

3. **branch**: main
4. **Make your changes**: Modify or add new features as needed.
5. **Submit a Pull Request**: Once your changes are ready, submit a pull request with a description of your changes.


### **Bugs/Error s**

**Observed Issue:**

- When the leader node receives a high volume of commit/reveal values from regular nodes simultaneously, it sometimes fails to store one or more values due to file I/O contention or other concurrency issues.

**Potential Causes:**

- High concurrency: too many simultaneous writes leading to missed file entries.
- Lack of a retry or confirmation mechanism.

**Suggested Improvements:**

1. **Post-Wait Retry Mechanism (Leader-Initiated):**
    - **Process:**
        1. Leader waits after receiving the bulk of CVS/COS/secret values.
        2. If any value is missing, leader requests that specific regular node to resend it.
        3. Upon receiving the missing value, the leader stores it.
        4. If missing values aren’t received after multiple retries, the leader may omit the node from the round, abort the round, or use another fallback.
2. **Rate Limiting at Regular Nodes (Regular Node-Initiated):**
    - **Process:**
        - Implement a brief waiting period (e.g., 5–10 seconds) before a regular node sends another value (CVS, COS, or secret) to the leader node.
        - This reduces the risk of overwhelming the leader node’s I/O operations and ensures more orderly handling of incoming data.

**Benefits:**

- **Data Integrity:** Both approaches help ensure that all values are accurately stored.
- **System Reliability:** Introducing waiting periods and retry mechanisms improves fault tolerance and system stability.
- **Scalability:** As the network grows and more regular nodes become active, these measures help maintain system responsiveness and correctness.