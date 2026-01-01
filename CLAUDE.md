# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based Distributed Random Beacon (DRB) Node implementation that operates in a peer-to-peer network using a commit-reveal cryptographic protocol. The system consists of Leader Nodes and Regular Nodes that collaborate to generate cryptographically secure random numbers on-chain.

## Development Commands

### Building and Running
- `./build.sh` - Main build script that starts all services (requires `static-key/leadernode.bin`)
- `./run_generator.sh` - Generate leader node peer ID (creates `static-key/leadernode.bin`)
- `docker compose up -d --build` - Start all nodes in Docker environment
- `docker compose down -v` - Stop and clean up all containers

### Testing
- `./run_leader_tests.sh` - Run leader node tests with test database
- `./run_regular_tests.sh` - Run regular node tests with test database  
- `./run_database_test.sh` - Run database layer tests
- `./run_utils_test.sh` - Run utility function tests
- `./run_commit-reveal2_test.sh` - Run commit-reveal protocol tests
- `./run_geth_test.sh` - Run integration tests with Geth
- `go test -v ./integration_test/...` - Run integration tests

### Individual Node Management
- `./run_leader.sh` - Start leader node only
- `./run_regular.sh` - Start regular node only
- `./start_drb_nodes.sh` - Start all DRB nodes
- `./stop_drb_nodes.sh` - Stop all DRB nodes

### Development Tools
- `go build -o main cmd/nodes/main.go` - Build main binary
- `go mod tidy` - Clean up dependencies
- Docker logs: `docker logs -f leadernode`, `docker logs -f regularnode1`, etc.

## Architecture

### Core Components
- **Leader Node (`nodes/leader/`)**: Orchestrates the DRB protocol, manages node registration, processes commitments, generates Merkle roots, and coordinates random number generation
- **Regular Node (`nodes/regular/`)**: Participates in the protocol by submitting commitments, revealing secrets in sequence, and maintaining P2P connectivity with the leader
- **Commit-Reveal Protocol (`commit-reveal2/`)**: Implements the cryptographic commitment scheme, Merkle tree generation, and reveal ordering logic
- **Database Layer (`database/`)**: PostgreSQL-based persistence with migration support for commits, node registry, and protocol state
- **P2P Networking (`libp2putils/`)**: LibP2P-based peer-to-peer communication between nodes
- **Ethereum Integration (`eth/`)**: Smart contract interactions, event monitoring, and transaction management with fallback RPC support

### Entry Points
- `cmd/nodes/main.go` - Main application entry point that initializes either leader or regular node based on `NODE_TYPE` environment variable
- `cmd/generator/main.go` - Utility to generate leader node peer ID

### Protocol Flow
1. **Registration**: Regular nodes register with leader and activate on-chain
2. **Event Monitoring**: Status events from smart contract trigger new DRB rounds
3. **Commitment Phase**: Regular nodes submit CVS (Commitment Value Signature) values  
4. **Merkle Root**: Leader generates and submits Merkle root on-chain
5. **Reveal Phase**: Sequential COS (Commitment Order Signature) and secret revelation
6. **Random Generation**: Final random number generation and round completion

### Database Schema
- Database migrations in `database/migrations/` with automatic schema management
- Core tables: leader_commits, regular_commits, peer_commits, node_info, reveal_order, batch_delete, broadcast_tracker
- Use `database.InitSQLDB()` for connection initialization

### Testing Strategy
- Unit tests for each component (`*_test.go` files)
- Integration tests with Docker containers and test databases
- Smart contract interaction tests with local Geth
- Test database cleanup handled automatically by test scripts

### Key Dependencies
- Go 1.23.0+ required
- Ethereum: `github.com/ethereum/go-ethereum v1.11.5`
- P2P: `github.com/libp2p/go-libp2p v0.37.1`
- Database: `github.com/go-pg/pg/v10 v10.14.0` and `github.com/lib/pq v1.10.9`
- Testing: `github.com/stretchr/testify v1.11.1`

### Environment Configuration
Required environment variables in `.env`:
- `NODE_TYPE` - "leader" or "regular"
- `LEADER_PRIVATE_KEY`, `EOA_PRIVATE_KEY_*` - Node private keys
- `ETH_RPC_URLS` - Comma-separated Ethereum RPC endpoints
- `CONTRACT_ADDRESS` - Deployed DRB contract address
- `POSTGRES_*` - Database connection parameters

### Docker Architecture
- Multi-container setup with separate PostgreSQL instances per node
- Health checks for database readiness and service availability
- Shared network (`anvil-net`) for inter-container communication
- Volume persistence for node keys and database data