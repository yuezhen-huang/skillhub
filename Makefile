.PHONY: all build proto clean

all: build

GO ?= go
PROTOC ?= protoc

proto:
	mkdir -p api/gen
	$(PROTOC) --go_out=. --go_opt=module=github.com/yuezhen-huang/skillhub \
		--go-grpc_out=. --go-grpc_opt=module=github.com/yuezhen-huang/skillhub \
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
