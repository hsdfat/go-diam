GO ?= go
BUF ?= buf
MODULES := . ./cmd/protoc-gen-go-diam ./internal/interop

.PHONY: generate check-generated test lint root-deps

generate:
	$(BUF) generate --template buf.gen.options.yaml --path proto/diam
	$(GO) build -o bin/protoc-gen-go-diam ./cmd/protoc-gen-go-diam
	$(BUF) generate --exclude-path proto/diam

check-generated: generate
	git diff --exit-code

test:
	@for m in $(MODULES); do (cd $$m && $(GO) test -race ./...) || exit 1; done

lint:
	$(BUF) lint
	@for m in $(MODULES); do (cd $$m && $(GO) vet ./...) || exit 1; done

# The root module must depend on nothing outside the standard library.
root-deps:
	@test "$$(GOWORK=off $(GO) list -m all)" = "github.com/hsdfat/go-diam" || \
		{ echo "root module has dependencies:"; GOWORK=off $(GO) list -m all; exit 1; }
