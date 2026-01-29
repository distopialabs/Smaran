# Samurai

## Introduction
...

## How to run
```
git clone https://github.com/distopialabs/Samurai.git
go mod tidy
go run ./cmd/samurai -mode commit -n 100
```

## How to generate performance graph
```
go install github.com/google/pprof@latest

sudo apt-get update
sudo apt-get install -y graphviz

go build -o samurai ./cmd/samurai
./samurai
go tool pprof -http=:8080 samurai ./profiles/cpu.prof
```


---


# Start the server
```
./samurai -mode serve -port 50051
```
# In another terminal, query with the client
```
./proofc -server localhost:50051 -account 0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2 -startBlock 20 -endBlock 119
```

## Build protobuf files
```
protoc --go_out=. --go_opt=paths=source_relative internal/tree/pb/segmenttree.proto
```