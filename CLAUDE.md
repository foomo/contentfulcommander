# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Contentful Commander — a Go library and CLI for Contentful CMA migrations. Provides a unified `Entity` interface over entries and assets, locale-aware field access, batch migration execution, and DeepL translation integration.

**Go 1.25** · module `github.com/foomo/contentfulcommander`

## Commands

```bash
make build        # Build binary to bin/contenfulcommander
make install      # Install to $GOPATH/bin
make test         # go test ./...
make lint         # golangci-lint run (40+ linters, 5m timeout)
make lint.fix     # golangci-lint run --fix

# Run a single test
go test ./commanderclient -run TestName
```

## Architecture

Single library package (`commanderclient/`) with a thin CLI entry point (`main.go`).

### Core abstractions

- **`Entity` interface** (`types.go`) — unified API over `EntryEntity` and `AssetEntity`. All field access is locale-aware with fallback support. Publishing status derived from version arithmetic: `draft` (PublishedVersion==0), `published` (Version-PublishedVersion==1), `changed` (>1).

- **`MigrationClient`** (`client.go`) — wraps the foomo/contentful CMA SDK. `LoadSpaceModel()` fetches locales, content types, entries, and assets concurrently into a `SpaceModel` cache. Provides entity lookups and filtering.

- **`EntityCollection`** (`collection.go`) — chainable operations on entity sets: filtering (50+ built-in filters), pagination, concurrent iteration (`ForEachConcurrent`), field extraction, grouping, stats, and conversion to migration operations.

- **`MigrationExecutor`** (`executor.go`) — runs `MigrationOperation` batches with configurable concurrency, dry-run mode, and per-operation confirmation. Update operations preserve publishing status.

### Translation subsystem

- **`DeepLTranslator`** (`translate.go`, `deepl.go`) — translates entity fields via DeepL API v2. Handles both simple strings and RichText (extracts text nodes, translates, reassembles). Tracks billed characters.
- **RichText processing** (`richtext_internal.go`, `hyperlinks.go`) — parses Contentful RichText JSON, extracts/replaces text nodes, processes hyperlinks.
- **Text utilities** (`textutils.go`) — `MatchCase`, `FixURI` (diacritics removal, slug generation), `ToLowerURL`.

### Configuration

Environment variables: `CONTENTFUL_CMAKEY`, `CONTENTFUL_SPACE_ID`, `CONTENTFUL_ENVIRONMENT`, `CONTENTFUL_VERBOSE`. Loaded via `LoadConfigFromEnv()` in `utils.go`.

## Conventions

- Local imports grouped under `github.com/foomo/contentfulcommander` (enforced by goimports)
- Max line length: 150 characters
- Tabs for indentation, trailing whitespace trimmed, LF line endings
- `golangci-lint` config in `.golangci.yml` — diagnostic and style tags enabled, performance/experimental/opinionated disabled
