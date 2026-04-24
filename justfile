# calemdar

set shell := ["bash", "-euo", "pipefail", "-c"]

bin := "./bin/calemdar"

# default: show recipes
default:
    @just --list

# build static binary into ./bin/
build:
    mkdir -p bin
    go build -o {{bin}} ./cmd/calemdar

# run with args (e.g. `just run series list`)
run *args:
    go run ./cmd/calemdar {{args}}

# install to GOPATH/bin
install:
    go install ./cmd/calemdar

# test everything
test:
    go test ./...

# test with race + verbose
test-v:
    go test -race -v ./...

# tidy deps
tidy:
    go mod tidy

# fmt + vet + test
check: tidy
    go fmt ./...
    go vet ./...
    go test ./...
