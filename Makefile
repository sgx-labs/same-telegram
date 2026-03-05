BINARY := same-telegram
VERSION := 0.1.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build clean test install

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BINARY) ./cmd/same-telegram/

install: build
	cp $(BINARY) $(GOPATH)/bin/ 2>/dev/null || cp $(BINARY) ~/go/bin/

test:
	CGO_ENABLED=1 go test ./...

clean:
	rm -f $(BINARY)

# Cross-compile for releases
.PHONY: release
release:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64 ./cmd/same-telegram/
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64 ./cmd/same-telegram/
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64 ./cmd/same-telegram/
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64 ./cmd/same-telegram/
