BINARY  := gloncher
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist

# Keep these names in sync with gloncher.sh, which looks up dist/gloncher-<os>-<arch>.
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build test vet fmt lint check release clean run

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

release: clean
	@mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=$(DIST)/$(BINARY)-$$os-$$arch; \
		[ "$$os" = "windows" ] && out=$$out.exe; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out . || exit 1; \
	done
	@cp gloncher.sh $(DIST)/
	@cd $(DIST) && ln -s gloncher.sh gloncher
	@ls -1 $(DIST)

run: build
	./$(BINARY) examples/demo.ini

clean:
	rm -rf $(DIST) $(BINARY)
