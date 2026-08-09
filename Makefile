
.PHONY: help test test-race cover cover-html fmt fmt-check fmt-md fmt-md-check vet build tidy tidy-all release-prep tag-modules provider-catalog-generate provider-catalog-check provider-watch-test check

COVER_PROFILE := cover.out

# Pinned so a Prettier release can never reformat the repo out from under CI.
# Bumping this is a deliberate change: bump, run `make fmt-md`, commit both.
PRETTIER := npx --yes prettier@3.9.6

# Tracked files only. Plain `find` also walks gitignored scratch — notably the
# repo copies under .claude/worktrees/ — which made the check scan 590 Markdown
# files instead of the 77 the repo actually owns.
GO_SOURCES := git ls-files '*.go'
MD_SOURCES := git ls-files '*.md'

help:
	@echo "Available targets:"
	@echo "  make test        - Run all tests"
	@echo "  make test-race   - Run tests with race detector"
	@echo "  make cover       - Run tests with coverage profile"
	@echo "  make cover-html  - Open HTML coverage report (writes $(COVER_PROFILE))"
	@echo "  make fmt         - Format all Go and Markdown files"
	@echo "  make fmt-md      - Format Markdown files with Prettier"
	@echo "  make fmt-check   - Fail if Go or Markdown files need formatting"
	@echo "  make fmt-md-check - Fail if Markdown files need formatting"
	@echo "  make vet         - Run go vet on all packages"
	@echo "  make tidy        - Tidy root module dependencies"
	@echo "  make tidy-all    - Tidy and format all modules"
	@echo "  make build       - Build the dive CLI"
	@echo "  make provider-catalog-generate - Generate provider Go files from catalog.json"
	@echo "  make provider-catalog-check - Verify provider catalogs and generated Go files"
	@echo "  make provider-watch-test - Run the provider watcher unit tests"
	@echo "  make release-prep VERSION=v1.0.0 - Point sub-module requirements at VERSION"
	@echo "  make tag-modules VERSION=v1.0.0 - Tag all sub-modules"
	@echo "  make check       - Run catalog checks, watcher tests, fmt-check, vet, and test"

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
	gofmt -w $$($(GO_SOURCES))
	$(PRETTIER) --write $$($(MD_SOURCES))

fmt-check:
	@test -z "$$(gofmt -l $$($(GO_SOURCES)))" || \
		(echo "The following files need gofmt:" && \
		gofmt -l $$($(GO_SOURCES)) && exit 1)
	$(PRETTIER) --check $$($(MD_SOURCES))

fmt-md:
	$(PRETTIER) --write $$($(MD_SOURCES))

fmt-md-check:
	$(PRETTIER) --check $$($(MD_SOURCES))

vet:
	go vet ./...

GO_MODULES := . providers/google providers/openai providers/grok a2a otel experimental/mcp experimental/cmd/dive examples

tidy:
	go mod tidy

tidy-all:
	@for dir in $(GO_MODULES); do \
		echo "==> $$dir"; \
		(cd $$dir && go mod tidy && gofmt -w $$($(GO_SOURCES))); \
	done

build:
	cd experimental/cmd/dive && go build .

SUB_MODULES := providers/google providers/openai providers/grok a2a otel experimental/mcp experimental/cmd/dive examples

release-prep:
ifndef VERSION
	$(error VERSION is required. Usage: make release-prep VERSION=v1.0.0)
endif
	python3 scripts/sync_module_versions.py $(VERSION) --write
	@echo ""
	@echo "Commit the go.mod updates, then run: make tag-modules VERSION=$(VERSION)"

tag-modules:
ifndef VERSION
	$(error VERSION is required. Usage: make tag-modules VERSION=v1.0.0)
endif
	@python3 scripts/sync_module_versions.py $(VERSION) --check
	@for mod in $(SUB_MODULES); do \
		echo "Tagging $$mod/$(VERSION)"; \
		git tag "$$mod/$(VERSION)"; \
	done
	@echo ""
	@echo "Tags created. Push with: git push origin --tags"

provider-catalog-generate:
	python3 scripts/generate_provider_catalogs.py

# Requires gofmt on PATH; the watcher tests do not.
provider-catalog-check:
	python3 scripts/generate_provider_catalogs.py --check

provider-watch-test:
	python3 -B -m unittest -v scripts/test_provider_watch.py

check: provider-catalog-check provider-watch-test fmt-check vet test
