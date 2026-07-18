//go:build tools

package tools

import (
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)

const (
	ProtocVersion          = "29.2"
	ProtocGenGoVersion     = "v1.36.11"
	ProtocGenGoGRPCVersion = "1.6.1"
)
