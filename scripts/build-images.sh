#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Building CEE runner images..."

docker build -t cee/python:3.8 "$PROJECT_DIR/docker/images/python/"
docker build -t cee/node:18 "$PROJECT_DIR/docker/images/node/"
docker build -t cee/gcc:9 "$PROJECT_DIR/docker/images/gcc/"
docker build -t cee/java:17 "$PROJECT_DIR/docker/images/java/"
docker build -t cee/golang:1.22 "$PROJECT_DIR/docker/images/golang/"
docker build -t cee/rust:latest "$PROJECT_DIR/docker/images/rust/"
docker build -t cee/multi:latest "$PROJECT_DIR/docker/images/multi/"

echo "All CEE runner images built successfully."
docker images | grep cee/

