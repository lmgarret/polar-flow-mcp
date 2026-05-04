.PHONY: build test lint clean

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/polar-flow-mcp ./cmd/polar-flow-mcp

test:
	CGO_ENABLED=0 go test -race -count=1 ./...

lint:
	~/go/bin/golangci-lint run ./...

clean:
	rm -rf bin/
