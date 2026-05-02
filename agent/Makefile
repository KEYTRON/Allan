BINARY := bin/allan
PKG := ./cmd/allan
VERSION := 0.1.0

.PHONY: build run install test clean tidy fmt vet

build:
	@mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) $(PKG)

run:
	go run $(PKG)

install:
	go install $(PKG)

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin/
