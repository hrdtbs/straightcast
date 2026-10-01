.PHONY: test lint build

test:
	go test -race ./cmd/... ./internal/...

lint:
	golangci-lint run

build:
	go build -trimpath -o straightcast ./cmd/straightcast

build-all:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/straightcast
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/straightcast
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o /dev/null ./cmd/straightcast
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o /dev/null ./cmd/straightcast
