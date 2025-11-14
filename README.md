# Samurai

## Introduction
...

## How to run
```
git clone https://github.com/distopialabs/Samurai.git
go mod tidy
go run ./cmd/samurai -mode commit -numBlocks 100
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

## Build protobuf files
```
protoc --go_out=. --go_opt=paths=source_relative internal/segmenttree/pb/segmenttree.proto
```

## Contributors:
- Asim Nepal
- Shistata Subedi
- Shubham Mishra
- Suyash Gupta
---
