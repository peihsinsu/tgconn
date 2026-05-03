VERSION    ?= dev
LDFLAGS     = -ldflags "-X github.com/cx009/tgconn/cmd.version=$(VERSION)"
IMAGE      ?= tgconn
INSTALL_DIR ?= $(HOME)/go/bin
DIST_DIR   ?= dist

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build build-all test lint clean install uninstall docker-build docker-run-session docker-run-apikey

build:
	go build $(LDFLAGS) -o tgconn .

test:
	go test ./...

lint:
	go vet ./...

build-all:
	$(foreach PLATFORM,$(PLATFORMS), \
	  $(eval OS   := $(word 1,$(subst /, ,$(PLATFORM)))) \
	  $(eval ARCH := $(word 2,$(subst /, ,$(PLATFORM)))) \
	  CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) \
	    go build $(LDFLAGS) -o $(DIST_DIR)/tgconn-$(OS)-$(ARCH) . ; \
	)
	@echo "built: $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/

clean:
	rm -f tgconn
	rm -rf $(DIST_DIR)/

install: build
	install -m 755 tgconn $(INSTALL_DIR)/tgconn
	@echo "installed: $(INSTALL_DIR)/tgconn"

uninstall:
	rm -f $(INSTALL_DIR)/tgconn
	@echo "removed: $(INSTALL_DIR)/tgconn"

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

# Auth via mounted ~/.claude session (OAuth login on host)
docker-run-session:
	docker run --rm \
	  -v ~/.claude:/root/.claude:ro \
	  -v $$(pwd):/workspace \
	  -e TELEGRAM_BOT_TOKEN \
	  $(IMAGE):latest --provider claude connect \
	    --allow-chat $(ALLOW_CHAT)

# Auth via Anthropic API key
docker-run-apikey:
	docker run --rm \
	  -v $$(pwd):/workspace \
	  -e TELEGRAM_BOT_TOKEN \
	  -e ANTHROPIC_API_KEY \
	  $(IMAGE):latest --provider claude connect \
	    --allow-chat $(ALLOW_CHAT)
