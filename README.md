# DRB Node Configuration Guide

This document provides comprehensive instructions for configuring and running a **Distributed Random Beacon (DRB)** node. The DRB node operates in two modes: **Leader Node** and **Regular Node**. Follow the setup steps according to the node type you intend to run.

---

## Prerequisites

Before setting up the DRB node, ensure the following requirements are met:

1. **Install Go**:  
   Ensure Go is installed on your system. **Go version 1.24.0 or greater** is required. [Refer to the Go installation guide](https://go.dev/doc/install) for details.
   
   Verify installation: `go version`
   

2. **Install Docker**:  
   Docker is required for running the nodes. [Refer to Docker installation guide](https://docs.docker.com/get-docker/) for details.

3. **Smart Contract Deployment**:  
   Deploy the DRB smart contract and obtain its address.  
   You can get the DRB smart contract from [here](https://github.com/tokamak-network/Commit-Reveal2/tree/audit/main-fixes).

4. **Account Balance**:  
   Ensure the Leader Node and Regular Node accounts have sufficient balance to perform transactions.
   - The **Leader Node** must have enough tokens to interact with the blockchain network, such as submitting Merkle roots and generating random numbers.
   - The **Regular Node** must have enough tokens to cover the deposit requirements set by the contract.

---

## Testing

The DRB Node project includes comprehensive unit tests and integration tests to ensure code quality and reliability. **All testing-related scripts and configurations are located in the `test/` directory**, which is separate from the deployment setup.

### Unit Testing

Unit tests verify individual components and functions in isolation. The test scripts automatically set up a test database using Docker Compose, run the tests, and clean up afterward.

#### Prerequisites for Unit Testing

Before running unit tests, ensure the following requirements are met:

1. **Docker** (Required):  
   Docker and Docker Compose are required for running unit tests. The test scripts use Docker Compose to set up test databases.
   - [Refer to Docker installation guide](https://docs.docker.com/get-docker/) for details.
   
   Verify installation:
   ```bash
   docker --version
   docker-compose --version
   docker compose version
   ```
   
   Expected output should show:
   ```
   Docker version 28.4.0, build d8eb465
   Docker Compose version v2.39.4-desktop.1
   Docker Compose version v2.39.4-desktop.1
   ```
   
   Ensure Docker is installed and running before executing unit tests.

2. **PostgreSQL** (Required):  
   PostgreSQL version 14.18 or greater is required for unit tests. The test scripts use Docker Compose to set up PostgreSQL test databases.
   - Verify Docker PostgreSQL container is running: The test scripts automatically start PostgreSQL containers via Docker Compose.

3. **Go** (Required):  
   Go version 1.24.0 or greater is required for running unit tests.
   
   Verify installation: `go version`
   

#### Running Unit Tests

**Using Shell Scripts** (from `test/` directory):

The project provides shell scripts in the `test/` directory for running unit tests for specific packages. These scripts automatically handle database setup and cleanup:

```bash
# Navigate to test directory
cd test/

# Test utils package
./run_utils_test.sh

# Test database package
./run_database_test.sh

# Test nodes/leader package
./run_leader_tests.sh

# Test nodes/regular package
./run_regular_tests.sh

# Test commit-reveal2 package
./run_commit-reveal2_test.sh
```

**Using Go Test Command Directly**:

You can also run tests directly using Go commands from the project root:

```bash
# Test specific packages (without coverage)
go test -v ./utils/...
go test -v ./database/...
go test -v ./nodes/leader/...
go test -v ./nodes/regular/...
go test -v ./commit-reveal2/...

# Test specific packages with coverage (generates coverage.out)
go test -v -coverprofile=coverage.out ./utils/...
go test -v -coverprofile=coverage.out ./database/...
go test -v -coverprofile=coverage.out ./nodes/leader/...
go test -v -coverprofile=coverage.out ./nodes/regular/...
go test -v -coverprofile=coverage.out ./commit-reveal2/...
```

**Generating Coverage Report**:

```bash
# Generate HTML coverage report from coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Integration Testing

Integration tests verify the interaction between multiple components, including Docker containers, blockchain nodes, and databases. These tests require additional setup and take longer to execute.

#### Prerequisites for Integration Testing

Before running integration tests, ensure the following requirements are met:

1. **Geth** (Required):  
   Geth version 1.16.3-stable or greater is required for integration tests. Geth is an Ethereum client used for integration tests. Install with:
   ```bash
   go install github.com/ethereum/go-ethereum/cmd/geth@latest
   ```
   Verify installation: `geth version`
   

2. **PostgreSQL** (Required):  
   PostgreSQL version 14.18 or greater is required for integration tests. Ensure PostgreSQL is running on `localhost:5432`.
   - **macOS**: `brew services start postgresql`
   - **Linux**: `sudo systemctl start postgresql`
   - Verify: `pg_isready -h localhost -p 5432`

3. **Docker** (Required):  
   Docker and Docker Compose are required for running Docker-based integration tests. Ensure Docker is installed and running.
   - [Refer to Docker installation guide](https://docs.docker.com/get-docker/) for details.
   
   Verify installation:
   ```bash
   docker --version
   docker-compose --version
   docker compose version
   ```
   
   Expected output should show:
   ```
   Docker version 28.4.0, build d8eb465
   Docker Compose version v2.39.4-desktop.1
   Docker Compose version v2.39.4-desktop.1
   ```

4. **Go** (Required):  
   Go version 1.24.0 or greater is required for running integration tests.
   
   Verify installation: `go version`

#### Running Integration Tests

Integration tests require Geth to be running first. Follow these steps:

**Step 1: Start Geth Node**

Start Geth in a separate terminal:

```bash
cd test/
./run_geth_test.sh
```

This will start Geth in development mode and keep it running. **Leave this terminal open** and keep Geth running while you execute the integration tests.

**Step 2: Run Integration Tests**

In a new terminal, run the integration tests:

```bash
# From project root
go test ./integration_test -v -timeout 120m
```

The integration test framework will:
1. Connect to the running Geth node
2. Generate peer IDs for leader and regular nodes
3. Create `.env.docker-test` file with all required configuration
4. Deploy smart contracts
5. Start all Docker containers
6. Run the tests
7. Clean up resources after completion

**Note**: 
- Integration tests use their own isolated environment and do not require or use the root `.env` file.
- Keep the Geth terminal running until all integration tests complete.
- Integration tests have a default timeout of 120 minutes.

#### Integration Test Details

Integration tests include:
- **Docker-based Node Testing**: Tests the full DRB node system with leader and regular nodes running in Docker containers
- **Blockchain Interaction Testing**: Verifies contract deployment, transactions, and event handling using Geth blockchain nodes
- **Database Testing**: Tests database operations with PostgreSQL instances
- **End-to-End Protocol Testing**: Validates the complete commit-reveal protocol flow


### Test Environment Setup

This section explains how to run your code in a test environment with all nodes (1 leader + 3 regular nodes) running together. 

**Note**: This is different from integration tests. Integration tests automatically configure everything. The test environment requires manual `.env` file configuration.

#### Prerequisites for Test Environment

Before setting up the test environment, ensure the following:

1. **Docker** (Required):  
   Docker and Docker Compose must be installed and running.

2. **PostgreSQL** (Required):  
   PostgreSQL version 14.18 or greater is required. The test environment uses Docker Compose to set up PostgreSQL instances automatically, so you don't need to install PostgreSQL separately on your system.

#### Steps to Run Test Environment

**Step 1: Generate Test Node Key Files**

Generate the required peer ID key files for testing:

```bash
cd test/

# Generate leader node key file
./run_leader_generator_test.sh

# Generate regular node key files (regularnode1.bin, regularnode2.bin, regularnode3.bin)
./run_regular_generator_test.sh
```

**Note**: If the key files already exist in `test/static-key/`, the scripts will not overwrite them. The leader node generator will skip generation if `leadernode.bin` exists, and the regular node generator will use existing files and only generate missing ones. To generate new peer IDs, delete the existing files first.

After generation, the scripts will output the peer IDs. **Save these peer IDs** as you'll need them for the `.env` file.

**Step 2: Configure .env File**

Configure your `.env` file with the test environment variables. See the [Test Environment Configuration](#test-environment-configuration) section for the required variables.

**Step 3: Build and Start Test Nodes**

Navigate to the test directory and run the build script:

```bash
cd test/
./build.sh
```

This script will:
- Check for required key files in `test/static-key/`
- Verify `.env` file exists in project root
- Build and start all test nodes (1 leader + 3 regular nodes) using Docker Compose
- Set up separate PostgreSQL instances for each node

**Step 4: Monitor Test Nodes**

View logs for each node:

```bash
# Leader node logs
docker logs -f leadernode

# Regular node 1 logs
docker logs -f regularnode1

# Regular node 2 logs
docker logs -f regularnode2

# Regular node 3 logs
docker logs -f regularnode3
```

**Step 5: Stop Test Environment**

To stop all test nodes:

```bash
cd test/
docker compose --env-file ../.env down
```

**Note**: The test environment uses:
- Test-specific Docker Compose configuration
- Multiple regular nodes running simultaneously for testing
- Test-specific peer ID files in `test/static-key/`
- Root `.env` file for configuration

For production deployment, see the [Deployment](#deployment) section.

---

## Test Environment Variables

This section describes the `.env` file configuration required for the test environment when running `test/build.sh` to test your code with all nodes.

**Important**: 
- Integration tests (`go test ./integration_test`) do NOT use the root `.env` file. They automatically create their own `.env.docker-test` file with all required configuration.
- For production deployment configuration, see the [Leader Node Configuration](#leader-node-configuration) and [Regular Node Configuration](#regular-node-configuration) sections under Deployment.

### Test Environment Configuration

When running the test environment (`test/build.sh`), configure your `.env` file with the following variables:

```bash

# Leader Node Configuration
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
LEADER_PORT=61280
LEADER_PEER_ID=<Generated from test/run_leader_generator_test.sh>

# Regular Nodes Configuration
REGULAR1_PEER_ID=<Generated from test/run_regular_generator_test.sh>
REGULAR1_PORT=61281
REGULAR2_PEER_ID=<Generated from test/run_regular_generator_test.sh>
REGULAR2_PORT=61282
REGULAR3_PEER_ID=<Generated from test/run_regular_generator_test.sh>
REGULAR3_PORT=61283

# Ethereum Configuration
ETH_RPC_URLS=<Your Ethereum RPC URLs separated by , (comma)>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
CHAIN_ID=<Chain_ID>

# Regular Node Private Keys (for test environment with 3 regular nodes)
EOA_PRIVATE_KEY_1=<Your Regular Node Private Key for Account 1>
EOA_PRIVATE_KEY_2=<Your Regular Node Private Key for Account 2>
EOA_PRIVATE_KEY_3=<Your Regular Node Private Key for Account 3>

# Database Configuration
POSTGRES_USER=postgres
POSTGRES_PASSWORD=password
```

**Note**: 
- The contract period configuration values must be set according to your `CHAIN_ID`. See the [Configuration by Chain ID](#configuration-by-chain-id) section below for detailed instructions.
- **For local/test environment**: The `STATUS` variable is **not required**. The node will automatically use local IP addresses.

#### Configuration by Chain ID

The contract period configuration variables control the timing windows for various operations in the DRB protocol. **You must add these variables to your `.env` file** based on your `CHAIN_ID`.

**Important**: Add the following environment variables to your `.env` file based on which network you are using:

**For ThanosSepolia (CHAIN_ID = 111551119090), Anvil (CHAIN_ID = 31337), or OpSepolia (CHAIN_ID = 11155111):**

If you are using ThanosSepolia, Anvil, or OpSepolia, **add the following to your `.env` file**:

```bash
OFF_CHAIN_SUBMISSION_PERIOD=40
REQUEST_OR_SUBMIT_OR_FAIL_DECISION_PERIOD=30
ON_CHAIN_SUBMISSION_PERIOD=60
OFF_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR=20
ON_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR=30
```

**For Sepolia (CHAIN_ID = 11155420):**

If you are using Sepolia, **add the following to your `.env` file**:

```bash
OFF_CHAIN_SUBMISSION_PERIOD=80
REQUEST_OR_SUBMIT_OR_FAIL_DECISION_PERIOD=60
ON_CHAIN_SUBMISSION_PERIOD=120
OFF_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR=20
ON_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR=40
```

> **Note:** `OFF_CHAIN_SUBMISSION_PERIOD_PER_OPERATOR` also controls the timeout for waiting for secret value responses from regular nodes. Adjust this value based on your chain's block time (e.g., lower for fast chains like Arbitrum, higher for slower chains).


## Deployment

**Deployment and testing are separated in this project.** All deployment-related scripts and configurations are located in the `deployment/` directory, which is separate from the testing setup.

### Deployment Structure

The deployment directory is organized as follows:

```
deployment/
├── leader/              # Leader node deployment
│   ├── build.sh         # Build and start leader node
│   ├── docker-compose.yml
│   ├── Dockerfile
│   ├── generate-peer-id.sh  # Generate leader peer ID
│   └── static-key/      # Leader node key storage
│       └── leadernode.bin
└── regular/             # Regular node deployment
    ├── build.sh         # Build and start regular node
    ├── docker-compose.yml
    ├── Dockerfile
    ├── generate-peer-id.sh  # Generate regular peer ID
    └── static-key/      # Regular node key storage
        └── regularnode.bin
```

### Prerequisites for Deployment

Before deploying nodes, ensure the following:

1. **Deploy the Smart Contract**:  
   Clone the repository for the DRB smart contract and deploy it to your preferred Ethereum network. Obtain the contract address. The leader node should be the owner of the contract, so deploy the contract with the leader node private key.
   - You can get the DRB smart contract from [here](https://github.com/tokamak-network/Commit-Reveal2/tree/audit/main-fixes).

2. **Create Docker Network** (Required):  
   The deployment uses an external Docker network `drb-production-net`. Create it before deploying:
   ```bash
   docker network create drb-production-net
   ```

3. **Configure Environment Status** (Required for Production):  
   Set the `STATUS` environment variable in your `.env` file based on your deployment environment:
   
   **For Production Deployment (AWS, Cloud Platforms):**
   ```bash
   STATUS=prod
   ```
   - Nodes will use public IP addresses for communication
   - Required when nodes are deployed on different networks/VPCs
   - Required for internet-based communication between nodes
   
   **For Local Development:**
   ```bash
   # Do NOT set STATUS variable - leave it unset
   ```
   - Nodes will automatically use local/Docker network IP addresses
   - No configuration needed for local testing
   - Works automatically with Docker Compose networks
   - **Important**: Do not add `STATUS` variable to your `.env` file for local development

### Deploying Leader Node

If you want to run a Leader Node on your system, follow these steps:

#### Leader Node Configuration

Configure your `.env` file in the project root with the following variables for Leader Node:

```bash
# Leader Node Configuration
LEADER_PRIVATE_KEY=<Your Leader Node Private Key>
LEADER_EOA=<Your Leader Ethereum Address>
LEADER_PORT=<Your Leader Node Port>
LEADER_PEER_ID=<Generated from deployment/leader/generate-peer-id.sh>

# Ethereum Configuration
ETH_RPC_URLS=<Your Ethereum RPC URLs separated by , (comma)>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
CHAIN_ID=<Chain_ID>

# Database Configuration
POSTGRES_USER=postgres
POSTGRES_PASSWORD=password

# Environment Status (Required for Production Deployment)
STATUS=prod

# Contract Period Configuration (based on CHAIN_ID)
```

**Note**: 
- The contract period configuration values must be set according to your `CHAIN_ID`. See the [Configuration by Chain ID](#configuration-by-chain-id) section for the correct values based on your network.
- **For production deployment**: Set `STATUS=prod` to use public IP addresses. This is required for nodes deployed on AWS or other cloud platforms where nodes need to communicate over the internet.
- **For local development**: Do **NOT** set the `STATUS` variable. Leave it unset in your `.env` file. The node will automatically use local/Docker network IP addresses.

#### Leader Node Deployment Steps

1. **Generate Leader Peer ID** (if not already generated):
   ```bash
   cd deployment/leader/
   ./generate-peer-id.sh
   ```
   This will create `deployment/leader/static-key/leadernode.bin` and output the `LEADER_PEER_ID`. Update the `LEADER_PEER_ID` in your `.env` file with the generated peer ID.
   
   **Note**: If `deployment/leader/static-key/leadernode.bin` already exists, the script will not generate a new peer ID. To generate a fresh peer ID, delete the existing file first.

2. **Build and Start Leader Node**:
   ```bash
   cd deployment/leader/
   ./build.sh
   ```
   This will:
   - Build the leader node Docker image
   - Start the leader node and its PostgreSQL database
   - Connect to the `drb-production-net` network

3. **View Leader Node Logs**:
   ```bash
   docker logs -f leadernode
   ```

**Note**: If you have permission issues with the scripts, make them executable:
```bash
chmod +x deployment/leader/build.sh
chmod +x deployment/leader/generate-peer-id.sh
```

### Deploying Regular Node

If you want to run a Regular Node on your system, follow these steps:

#### Regular Node Configuration

Configure your `.env` file in the project root with the following variables for Regular Node:

```bash
# Regular Node Configuration
LEADER_PEER_ID=<Leader Node Peer ID>
LEADER_EOA=<Leader Ethereum Address>
LEADER_PORT=<Leader Node Port>
PORT=<This Regular Node's Port>
EOA_PRIVATE_KEY=<This Regular Node's Private Key>
REGULAR_PEER_ID=<Generated from deployment/regular/generate-peer-id.sh>


# Ethereum Configuration
ETH_RPC_URLS=<Your Ethereum RPC URLs separated by , (comma)>
CONTRACT_ADDRESS=<Deployed DRB Contract Address>
CHAIN_ID=<Chain_ID>

# Database Configuration
POSTGRES_USER=postgres
POSTGRES_PASSWORD=<Your Production Database Password>

# Environment Status (Required for Production Deployment)
STATUS=prod

# Contract Period Configuration (based on CHAIN_ID)
```

**Note**: 
- The contract period configuration values must be set according to your `CHAIN_ID`. See the [Configuration by Chain ID](#configuration-by-chain-id) section for the correct values based on your network.
- **For production deployment**: Set `STATUS=prod` to use public IP addresses. This is required for nodes deployed on AWS or other cloud platforms where nodes need to communicate over the internet.
- **For local development**: Do **NOT** set the `STATUS` variable. Leave it unset in your `.env` file. The node will automatically use local/Docker network IP addresses.

#### Regular Node Deployment Steps

1. **Generate Regular Peer ID** (if not already generated):
   ```bash
   cd deployment/regular/
   ./generate-peer-id.sh
   ```
   This will create `deployment/regular/static-key/regularnode.bin`. The peer ID will be displayed.
   
   **Note**: If `deployment/regular/static-key/regularnode.bin` already exists, the script will load the existing peer ID and will not generate a new one. To generate a fresh peer ID, delete the existing file first.

2. **Build and Start Regular Node**:
   ```bash
   cd deployment/regular/
   ./build.sh
   ```
   This will:
   - Build the regular node Docker image
   - Start the regular node and its PostgreSQL database
   - Connect to the `drb-production-net` network

3. **View Regular Node Logs**:
   ```bash
   docker logs -f regularnode
   ```

**Note**: If you have permission issues with the scripts, make them executable:
```bash
chmod +x deployment/regular/build.sh
chmod +x deployment/regular/generate-peer-id.sh
```

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
│   ├── main.go                   # Main file to start the DRB node
│   ├── generator/                # Peer ID generator for leader node
│   │   └── main.go
│   └── regulargenerator/         # Peer ID generator for regular nodes
│       └── main.go
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
├── deployment/                   # Production deployment configurations
│   ├── leader/                   # Leader node deployment
│   │   ├── build.sh              # Build and start leader node script
│   │   ├── docker-compose.yml    # Docker Compose for leader node
│   │   ├── Dockerfile            # Leader node Docker configuration
│   │   ├── generate-peer-id.sh  # Generate leader peer ID script
│   │   └── static-key/           # Leader node key storage
│   │       └── leadernode.bin
│   └── regular/                  # Regular node deployment
│       ├── build.sh              # Build and start regular node script
│       ├── docker-compose.yml    # Docker Compose for regular node
│       ├── Dockerfile            # Regular node Docker configuration
│       ├── generate-peer-id.sh  # Generate regular peer ID script
│       └── static-key/           # Regular node key storage
│           └── regularnode.bin
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
├── test/                         # Testing configurations and scripts
│   ├── build.sh                  # Build all test nodes (1 leader + 3 regular)
│   ├── docker-compose.yml        # Docker Compose for test environment
│   ├── docker-compose.test.yml   # Docker Compose for unit tests
│   ├── Dockerfile                # Test Docker configuration
│   ├── run_commit-reveal2_test.sh # Test commit-reveal2 package
│   ├── run_database_test.sh      # Test database package
│   ├── run_geth_test.sh          # Start Geth for integration tests
│   ├── run_leader_generator_test.sh # Generate leader peer ID for testing
│   ├── run_leader_tests.sh       # Test nodes/leader package
│   ├── run_regular_generator_test.sh # Generate regular peer IDs for testing
│   ├── run_regular_tests.sh      # Test nodes/regular package
│   ├── run_utils_test.sh         # Test utils package
│   └── static-key/               # Test node key storage
│       ├── leadernode.bin
│       ├── regularnode1.bin
│       ├── regularnode2.bin
│       └── regularnode3.bin
├── utils/                        # Utility functions and helpers
│   ├── broadcast.go              # Broadcasting utilities and message structures
│   ├── clients.go                # Ethereum client setup and contract ABI loading
│   ├── commit.go                 # Commit data structures and management
│   ├── ip_retriever.go           # IP address retrieval utilities
│   ├── node_info.go              # Node information handling
│   ├── reveal_order.go           # Reveal order utilities
│   ├── streamHandler.go          # LibP2P stream handling
│   └── utils.go                  # General utility functions (signature verification, etc.)
├── integration_test/             # Integration test suite
│   ├── docker_nodes_quick_test.go # Docker-based integration tests
│   └── setup/                    # Test environment setup utilities
│       ├── test_setup.go         # Test environment configuration
│       └── geth_setup.go         # Geth blockchain node setup for tests
├── go.mod                        # Go module dependencies
├── go.sum                        # Go module checksums
├── .env                          # Environment variables configuration (not in repo)
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

### **Bugs/Errors**

**Observed Issue:**

- When the leader node receives a high volume of commit/reveal values from regular nodes simultaneously, it sometimes fails to store one or more values due to file I/O contention or other concurrency issues.
