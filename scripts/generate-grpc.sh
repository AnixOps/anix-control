#!/bin/bash

# 生成 gRPC 代码脚本

set -e

echo "Installing protoc plugins..."

# 安装 Go protobuf 插件
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

echo "Generating gRPC code..."

# 创建输出目录
mkdir -p api/grpc/v2boardpb

# 生成 Go 代码
protoc --go_out=./api/grpc/v2boardpb \
       --go_opt=paths=source_relative \
       --go-grpc_out=./api/grpc/v2boardpb \
       --go-grpc_opt=paths=source_relative \
       api/grpc/v2board.proto

echo "gRPC code generated successfully!"