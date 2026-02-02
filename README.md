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




## Count account updates
```
go run ./cmd/tools/count_account_updates -n 2616996 -start 18908895 -o account_stats.csv -dataset ./data/blocks
```
Metadata:
Time taken: 5m17.830670205s
Total unique accounts: 59,736,029
Total updates: 892,821,890

Output (head 10, sorted by UpdateCount desc):
Address,UpdateCount
0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2,2609815
0xdAC17F958D2ee523a2206206994597C13D831ec7,2609771
0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48,2588681
0x0000000000000000000000000000000000000001,2582745
0x3fC91A3afd70395Cd496C647d5a6CC9D4B2b7FAD,2553988
0x000000000022D473030F116dDEE9F6B43aC78BA3,2344863
0x000F3df6D732807Ef1319fB7B8bB8522d0Beac02,2099305
0xffffFFFfFFffffffffffffffFfFFFfffFFFfFFfE,2099304
0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D,2084319