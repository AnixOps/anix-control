#!/bin/bash

echo "Building Frontend..."
cd web
npm install
npm run build
cd ..

echo "Building Backend..."
go build -o v2board ./cmd/server

echo "Build Complete!"
