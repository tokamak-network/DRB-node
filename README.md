Replace PeerID in .env
PEER_ID=node1/node2/node3
PORT=61291/61292/61293


# node-1
go run node.go node1

# node-2
go run node.go node2 /ip4/<ip>/tcp/<port>/p2p/<peerID>

# node-3
go run node.go node3 /ip4/<ip>/tcp/<port>/p2p/<peerID>
