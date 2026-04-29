.PHONY: all build proto clean

all: build

GO ?= go
PROTOC ?= protoc

proto:
	mkdir -p api/gen
	$(PROTOC) --go_out=api/gen --go_opt=paths=source_relative \
		--go-grpc_out=api/gen --go-grpc_opt=paths=source_relative \
		-I api/proto api/proto/*.proto

build: proto
	$(GO) build -o bin/skillhub ./cmd/skillhub
	$(GO) build -o bin/skillctl ./cmd/skillctl

install: proto
	$(GO) install ./cmd/skillhub
	$(GO) install ./cmd/skillctl

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin api/gen
