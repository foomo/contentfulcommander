.DEFAULT_GOAL:=help
-include .makerc

# --- Config ------------------------------------------------------------------

# Newline hack for error output
define br


endef

# --- Targets -----------------------------------------------------------------

# This allows us to accept extra arguments
%: .mise .lefthook
	@:

.PHONY: .mise
# Install dependencies
.mise:
ifeq (, $(shell command -v mise))
	$(error $(br)$(br)Please ensure you have 'mise' installed and activated!$(br)$(br)  $$ brew update$(br)  $$ brew install mise$(br)$(br)See the documentation: https://mise.jdx.dev/getting-started.html)
endif
	@mise install

# Configure git hooks for lefthook
.lefthook:
	@lefthook install --reset-hooks-path

### Tasks

.PHONY: check
## Run lint & tests
check: tidy generate lint test

.PHONY: tidy
## Run go mod tidy
tidy:
	@echo "〉go mod tidy"
	@go mod tidy

.PHONY: lint
## Run linter
lint:
	@echo "〉golangci-lint run"
	@golangci-lint run

.PHONY: lint.fix
## Run golangci-lint & fix
lint.fix:
	@echo "〉golangci-lint run fix"
	@golangci-lint run --fix

.PHONY: generate
## Run go generate
generate:
	@echo "〉go generate"
	@go generate ./...

.PHONY: test
## Run go tests
test:
	@echo "〉go test"
	@GO_TEST_TAGS=-skip go test -tags=safe -coverprofile=coverage.out work

.PHONY: test.race
## Run go tests with `race` flag
test.race:
	@echo "〉go test with -race"
	@GO_TEST_TAGS=-skip go test -tags=safe -coverprofile=coverage.out -race work

.PHONY: test.update
## Run go tests with `update` flag
test.update:
	@echo "〉go test with -update"
	@GO_TEST_TAGS=-skip go test -tags=safe -coverprofile=coverage.out -update work

.PHONY: outdated
## Show outdated direct dependencies
outdated:
	@echo "〉go mod outdated"
	@go list -u -m -json all | go-mod-outdated -update -direct


.PHONY: install
## Install binary
install:
	@go build -tags=safe -o ${GOPATH}/bin/contenfulcommander main.go

.PHONY: build
## Build binary
build:
	@mkdir -p bin
	@go build -tags=safe -o bin/contenfulcommander main.go

### Documentation

.PHONY: godocs
## Open go docs
godocs:
	@echo "〉starting go docs"
	@go doc -http

### Utils

.PHONY: help
## Show help text
help:
	@echo "Contentfulcommander\n"
	@echo "Usage:\n  make [task]"
	@awk '{ \
		if($$0 ~ /^### /){ \
			if(help) printf "%-23s %s\n\n", cmd, help; help=""; \
			printf "\n%s:\n", substr($$0,5); \
		} else if($$0 ~ /^[a-zA-Z0-9._-]+:/){ \
			cmd = substr($$0, 1, index($$0, ":")-1); \
			if(help) printf "  %-23s %s\n", cmd, help; help=""; \
		} else if($$0 ~ /^##/){ \
			help = help ? help "\n                        " substr($$0,3) : substr($$0,3); \
		} else if(help){ \
			print "\n                        " help "\n"; help=""; \
		} \
	}' $(MAKEFILE_LIST)

