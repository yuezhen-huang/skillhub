# Setup Guide

## Prerequisites

- Go 1.21+
- protoc (Protocol Buffers compiler)
- protoc-gen-go
- protoc-gen-go-grpc

## Install Protobuf Tools

```bash
# Install protoc (macOS)
brew install protobuf

# Or download from https://github.com/protocolbuffers/protobuf/releases

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

## Build Steps

```bash
# Generate protobuf code
make proto

# Tidy dependencies
go mod tidy

# Build
make build
```

## Alternative: Quick Test Without Protobuf

If you want to test the core logic without gRPC first, you can use the test script:

```bash
go run test_simple.go
```
