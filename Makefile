VERSION ?= dev
LDFLAGS  = -ldflags "-X github.com/cx009/tgconn/cmd.version=$(VERSION)"

.PHONY: build test lint clean

build:
	go build $(LDFLAGS) -o tgconn .

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f tgconn
