.PHONY: build test lint skill clean

build:
	CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/polar-flow-mcp ./cmd/polar-flow-mcp

test:
	CGO_ENABLED=0 go test -count=1 ./...

lint:
	~/go/bin/golangci-lint run ./...

# Claude skill bundle (a zip with polar-flow/SKILL.md at its root), attached to
# GitHub releases for upload in Claude.ai or unzip into ~/.claude/skills/.
skill:
	mkdir -p bin
	rm -f bin/polar-flow.skill
	cd skill && zip -qrX ../bin/polar-flow.skill polar-flow

clean:
	rm -rf bin/
