# DRB-node

# Without Docker
go run cmd/main.go

# With Docker file
docker build -t drb-node . 

docker run --rm -it drb-node

# With Docker compose
docker-compose up --build

# With Docker execute
docker-compose up -d
