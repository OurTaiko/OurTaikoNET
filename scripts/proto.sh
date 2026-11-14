#!/bin/sh
# Regenerates the Go code of proto/ (internal/multiplay/multiplaypb). The result is committed:
# the Docker build has no protoc.
#
# Needs protoc and the two Go plugins on PATH or in $(go env GOPATH)/bin, in the versions the
# generated files name in their headers:
#
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
set -eu
cd "$(dirname "$0")/.."
PATH="$(go env GOPATH)/bin:$PATH"
export PATH
for tool in protoc protoc-gen-go protoc-gen-go-grpc; do
  command -v "$tool" >/dev/null || { echo "proto.sh: $tool is not on PATH" >&2; exit 1; }
done
protoc -I proto \
  --go_out=. --go_opt=module=ourtaiko.dev/net \
  --go-grpc_out=. --go-grpc_opt=module=ourtaiko.dev/net \
  ourtaiko/multiplay/v1/multiplay.proto
gofmt -w internal/multiplay/multiplaypb
