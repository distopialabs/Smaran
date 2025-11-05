.PHONY: all
all:
	mkdir -p build
	mkdir -p profiles
	go build -o build ./cmd/...