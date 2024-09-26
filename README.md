# DRB-node
The DRB-node is a tool for interacting with the DRB Coordinator on Ethereum-like networks. Follow the instructions below to configure your setup, adjust environment variables, and run the node using Go, Docker, or Docker Compose.

-------------------------------------------------------------------------------------------------------

# Configuration
Create a config.json file with network-specific details. Adjust the values as per your network requirements:

```
{
  "RpcURL": "wss://<YOUR_NETWORK_RPC>/ws",
  "HttpURL": "https://<YOUR_NETWORK_RPC>",
  "ContractAddress": "0x<YOUR_CONTRACT_ADDRESS>",
  "SubgraphURL": "https://<YOUR_SUBGRAPH_URL>",
  "OperatorDepositFee": "<YOUR_OPERATOR_DEPOSIT_FEE>"
}
```

`<RpcURL>`: WebSocket URL for connecting to your desired Ethereum network.<br>
`<HttpURL>`: HTTP URL for interacting with the network.<br>
`<ContractAddress>`: Address of the DRB Coordinator contract deployed on the network.<br>
`<SubgraphURL>`: URL for querying data through a subgraph, if applicable.<br>
`<OperatorDepositFee>`: Fee required for operators to deposit (in wei or other relevant unit).<br>

You can replace the placeholders (`<YOUR_NETWORK_RPC>`, `<YOUR_CONTRACT_ADDRESS>`, etc.) with the specific values for your setup.

-------------------------------------------------------------------------------------------------------

# Environment Variables

Create a .env file and fill in your personal wallet information:

```
PRIVATE_KEY="YOUR_PRIVATE_KEY"
WALLET_ADDRESS="YOUR_WALLET_ADDRESS"
```

`<PRIVATE_KEY>`: Your Ethereum private key for signing transactions.<br>
`<WALLET_ADDRESS>`: Ethereum wallet address linked to the private key.<br>

Important: Keep the .env file secure and avoid sharing your private key publicly.

-------------------------------------------------------------------------------------------------------

# Running the Node

You can run the node in three different ways: using Go, Docker, or Docker Compose.

# 1. Without Docker

To run the node directly using Go, execute the following command:

`<Without Build>`<br>
go run cmd/main.go

`<With Build>`<br>
go build cmd/main.go

./main

# 2. With Docker

For Docker users:

`<Build the Docker image:>`<br>
docker build -t drb-node .

`<Run the Docker container:>`<br>
docker run --rm -it drb-node

# 3. With Docker Compose

For using Docker Compose:

`<Build and start the node:>`<br>
docker-compose up --build

`<To run in detached mode:>`<br>
docker-compose up -d

-------------------------------------------------------------------------------------------------------

# Additional Notes
`<Network Flexibility:>` The values in config.json can be adjusted to connect to any Ethereum-based network (e.g., Sepolia, Mainnet, Rinkeby, etc.).

`<Environment Security:>` Always keep your .env file safe, especially when sharing the project or pushing it to repositories.




