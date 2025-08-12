# Samurai

## Introduction
...

## How to run
```
git clone https://github.com/distopialabs/samurai.git
go mod tidy
go run main.go
```

## How to generate performance graph
```
go install github.com/google/pprof@latest

sudo apt-get update
sudo apt-get install -y graphviz

go build -o samurai .
./samurai
go tool pprof -http=:8080 samurai cpu.prof
```


---

## Contributors:
- Asim Nepal
- Shistata Subedi
- Shubham Mishra
- Suyash Gupta
---
