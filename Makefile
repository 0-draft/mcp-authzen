BIN := mcp-authzen
PKG := ./...

.PHONY: build test vet lint smoke clean fmt tidy

build:
	go build -trimpath -ldflags "-s -w" -o $(BIN) .

test:
	go test -race -count=1 -v $(PKG)

vet:
	go vet $(PKG)

lint:
	golangci-lint run $(PKG)

smoke: build
	./scripts/smoke.sh ./$(BIN)

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -f $(BIN)
	rm -rf dist/
