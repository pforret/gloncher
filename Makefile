BINARY  := gloncher
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist
# Prebuilt binaries committed to the repo, so `basher install` and a plain
# clone work without a Go toolchain. Rebuild with `make binaries` on release.
BINDIR  := binaries

# Keep these names in sync with gloncher.sh, which looks up dist/gloncher-<os>-<arch>.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build test vet fmt lint check release binaries clean run

all: check build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

lint:
	@test -z "$$(gofmt -l .)" || { echo "not gofmt'd:"; gofmt -l .; exit 1; }

check: lint vet test

binaries:
	@mkdir -p $(BINDIR)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=$(BINDIR)/$(BINARY)-$$os-$$arch; \
		[ "$$os" = "windows" ] && out=$$out.exe; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out . || exit 1; \
	done
	@du -sh $(BINDIR)

release: clean
	@mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=$(DIST)/$(BINARY)-$$os-$$arch; \
		[ "$$os" = "windows" ] && out=$$out.exe; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out . || exit 1; \
	done
	@cp bin/gloncher $(DIST)/gloncher.sh
	@cd $(DIST) && ln -sf gloncher.sh gloncher
	@ls -1 $(DIST)

run: build
	./$(BINARY) examples/demo.ini

clean:
	rm -rf $(DIST) $(BINARY) $(BINARY)-*-*
