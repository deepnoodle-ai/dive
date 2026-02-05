
.PHONY: help test test-race cover cover-html fmt fmt-check fmt-md fmt-md-check vet build tidy check

COVER_PROFILE := cover.out

help:
	@echo "Available targets:"
	@echo "  make test        - Run all tests"
	@echo "  make test-race   - Run tests with race detector"
	@echo "  make cover       - Run tests with coverage profile"
	@echo "  make cover-html  - Open HTML coverage report (writes $(COVER_PROFILE))"
	@echo "  make fmt         - Format all Go files"
	@echo "  make fmt-md      - Format Markdown files with Prettier"
	@echo "  make fmt-check   - Fail if Go or Markdown files need formatting"
	@echo "  make fmt-md-check - Fail if Markdown files need formatting"
	@echo "  make vet         - Run go vet on all packages"
	@echo "  make tidy        - Tidy module dependencies"
	@echo "  make build       - Build the dive CLI"
	@echo "  make check       - Run fmt-check, vet, and test"

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile $(COVER_PROFILE) ./...
	go tool cover -func $(COVER_PROFILE)

cover-html:
	go test -coverprofile $(COVER_PROFILE) ./...
	go tool cover -html=$(COVER_PROFILE)

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')
	npx --yes prettier --write $$(find . -name '*.md' -not -path './.git/*')

fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './.git/*'))" || \
		(echo "The following files need gofmt:" && \
		gofmt -l $$(find . -name '*.go' -not -path './.git/*') && exit 1)
	npx --yes prettier --check $$(find . -name '*.md' -not -path './.git/*')

fmt-md:
	npx --yes prettier --write $$(find . -name '*.md' -not -path './.git/*')

fmt-md-check:
	npx --yes prettier --check $$(find . -name '*.md' -not -path './.git/*')

vet:
	go vet ./...

tidy:
	go mod tidy

build:
	go build ./cmd/dive

check: fmt-check vet test
