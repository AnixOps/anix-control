#!/bin/bash

echo "Building Frontend..."
cd web
npm install
npm run build
cd ..

echo "Building Backend..."
mkdir -p build
GOWORK=off go build -o build/v2board ./cmd/server

echo "Build Complete!"
