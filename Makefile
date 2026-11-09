B=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
REV=$(B)-$(shell date +%Y%m%d)
LDFLAGS=-s -w -X main.revision=$(REV)

.PHONY: build run test lint docker

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o dummypage ./app

run:
	go run ./app --dbg

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	golangci-lint run ./... || true

docker:
	docker build -t dummypage:$(REV) .
