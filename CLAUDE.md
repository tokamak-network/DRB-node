# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Build and Run
- `./build.sh` - Main build and deployment script. Requires `static-key/leadernode.bin` file
- `./run_generator.sh` - Generate leader node peer ID. Creates `static-key/leadernode.bin` and outputs LEADER_PEER_ID for .env
- `docker compose up -d --build` - Build and run all nodes in Docker containers
- `docker logs -f leadernode` - View leader node logs
- `docker logs -f regularnode1/2/3` - View regular node logs

### Testing
- `go test -v ./commit-reveal2/...` - Run commit-reveal2 module tests
- `go test -v ./database/...` - Run database layer tests  
- `go test -v ./utils/...` - Run utility function tests
- `go test -v ./nodes/leader/...` - Run leader node tests
- `go test -v ./nodes/regular/...` - Run regular node tests
- `go test -race -v ./...` - Run all tests with race condition detection
- `./run_race_tests.sh` - Comprehensive race condition testing suite
- `./run_commit-reveal2_test.sh` - Run commit-reveal2 tests with test database

### Individual Test Scripts
- `./run_database_test.sh` - Database tests with PostgreSQL setup
- `./run_utils_test.sh` - Utility function tests
- `./run_leader_tests.sh` - Leader node specific tests
- `./run_regular_tests.sh` - Regular node specific tests
- `./run_geth_test.sh` - Ethereum integration tests

## Project Architecture

### Core Node Types
- **Leader Node**: Coordinates DRB protocol, processes commitments, generates Merkle roots, manages reveal order
- **Regular Node**: Participates in commitment protocol, submits CVS/COS values, reveals secrets sequentially

### Key Architectural Components

#### Database Layer (`database/`)
- PostgreSQL-based persistence with migration system
- Repositories: `LeaderCommitRepository`, `RegularCommitRepository`, `NodeInfoRepository`, `RevealOrderRepository`
- Batch operations and broadcast tracking

#### P2P Communication (`libp2putils/`)
- LibP2P-based peer-to-peer networking
- Stream handling for node-to-node communication
- Leader-regular node registration and message broadcasting

#### Blockchain Integration (`eth/`, `pkg/fallback_ethclient/`)
- Fallback RPC client with automatic failover
- Smart contract interaction via generated Go bindings
- Event monitoring and transaction handling

#### Commit-Reveal Protocol (`commit-reveal2/`)
- Multi-phase cryptographic commitment process
- Merkle tree generation for commitment verification
- Sequential revelation order management

### Node Structure

#### Leader Node (`nodes/leader/`)
Key files:
- `leader_node.go` - Main leader implementation with atomic state management
- `accept_commit.go` - Event processing and monitoring logic
- `monitor_commits.go` - Round completion and validation
- `reveal_requests.go` - Sequential secret value request management
- `registration_helper.go` - Node registration and activation

#### Regular Node (`nodes/regular/`)
Key files:
- `regular_node.go` - Main regular node implementation
- `send_commit.go` - Commitment submission with event monitoring
- `secret_handler.go` - Secret value revelation handling
- `csv_signature.go` - CVS signature generation

### Concurrency and Thread Safety
- Extensive use of atomic operations for state flags
- Mutex protection for shared data structures
- Race condition testing with `./run_race_tests.sh`
- Time-based monitoring with automatic retry mechanisms

## Environment Setup

### Prerequisites
- Go 1.23.0+ (toolchain auto-management enabled)
- Docker and Docker Compose
- PostgreSQL (for local development)

### Required Environment Variables (.env)
Leader node:
```
LEADER_PRIVATE_KEY=<private_key>
LEADER_EOA=<ethereum_address>
ETH_RPC_URLS=<rpc_urls_comma_separated>
CONTRACT_ADDRESS=<deployed_contract_address>
POSTGRES_PASSWORD=password
```

Regular node (additional):
```
LEADER_PEER_ID=<from_run_generator.sh>
EOA_PRIVATE_KEY_1/2/3=<regular_node_private_keys>
CHAIN_ID=111551119090
```

## Development Workflow

1. **Initial Setup**: Run `./run_generator.sh` to generate leader peer ID
2. **Configuration**: Update .env with LEADER_PEER_ID and other required values
3. **Build & Deploy**: Use `./build.sh` to build and run all nodes
4. **Testing**: Run race condition tests regularly with `./run_race_tests.sh`
5. **Monitoring**: Use Docker logs to monitor node operations

## Key Protocol Flow
1. Node registration and on-chain activation
2. Round initialization via blockchain events
3. CVS commitment phase with time-based monitoring
4. Merkle root submission and COS handling
5. Sequential secret revelation
6. Random number generation and round completion

## Testing Strategy
- Unit tests for each component
- Integration tests with Docker compose
- Race condition detection with stress testing
- Database migration and cleanup testing
- Concurrency stress tests in `testing/5_concurrency_advanced/`