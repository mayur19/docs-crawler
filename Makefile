.PHONY: build test lint vet clean

BINARY := docs-crawler

build:
	go build -o $(BINARY) .

test:
	go test ./... -count=1 -race

test-cover:
	go test ./... -coverprofile=cover.out
	go tool cover -func=cover.out

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY) cover.out
