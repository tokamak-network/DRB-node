# DRB-Node

# Without Docker
go run cmd/main.go

# With Docker file
docker build -t go-app . 
docker run --rm -it go-app

# With Docker compose
docker-compose up --build
