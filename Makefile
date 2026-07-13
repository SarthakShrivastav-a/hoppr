.PHONY: build install test race fmt vet lint clean run

BIN := hoppr
PKG := ./cmd/hoppr

build:
	go build -trimpath -o bin/$(BIN) $(PKG)

install:
	go install $(PKG)

test:
	go test ./...

race:
	go test -race ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# What CI runs, so a green `make lint` locally means a green CI.
lint: fmt vet
	go mod tidy
	git diff --exit-code go.mod go.sum

run: build
	./bin/$(BIN)

clean:
	rm -rf bin dist
