default:
    @just --list

# build the didban binary for this machine
build:
    go build -o didban ./cmd/didban

# cross-build for a Raspberry Pi 4 (linux/arm64)
build-pi:
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o didban ./cmd/didban

# update go packages
update:
    go get -u ./...
    go mod tidy

# run tests
test:
    go test -race -v ./... -covermode=atomic -coverprofile=coverage.out

# run golangci-lint
lint:
    golangci-lint run -c .golangci.yml

tidy:
    go mod tidy
