.PHONY: build install test format clean

BINARY=retry
PREFIX?=/usr/local
BINDIR=$(PREFIX)/bin

build:
	go build -ldflags="-s -w" -o $(BINARY) .

install: build
	install -d $(BINDIR)
	install -m 755 $(BINARY) $(BINDIR)/$(BINARY)

test:
	go test -v ./...

format:
	go fmt ./...

clean:
	rm -f $(BINARY)