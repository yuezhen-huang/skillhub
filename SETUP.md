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

# Install Go plugins (protoc Go generators)
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0

# Ensure GOPATH/bin is in PATH (required for make proto / make build)
export PATH="$(go env GOPATH)/bin:$PATH"

# Optional: persist it for zsh
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
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
