# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Contentful Commander — a Go library and CLI for Contentful CMA migrations. Provides a unified `Entity` interface over entries and assets, locale-aware field access, dual CMA/CDA loading with CDA views, batch migration execution, and DeepL translation integration.

**Go 1.25** · module `github.com/foomo/contentfulcommander`

## Tooling

- **mise** (`.mise.toml`) — manages tool versions: lefthook, golangci-lint
- **lefthook** (`.lefthook.yaml`) — git hooks: pre-commit (branch naming `feature/`|`fix/`, golangci fmt/lint), commit-msg (conventional commits), post-checkout (mise install)
- **dependabot** (`.github/dependabot.yml`) — automated dependency updates for GitHub Actions and Go modules

## Commands

```bash
make check        # Run tidy, generate, lint, test (full CI check)
make build        # Build binary to bin/contenfulcommander (with -tags=safe)
make install      # Install to $GOPATH/bin (with -tags=safe)
make test         # go test -tags=safe -coverprofile=coverage.out
make test.race    # go test with -race flag
make test.update  # go test with -update flag
make lint         # golangci-lint run
make lint.fix     # golangci-lint run --fix
make tidy         # go mod tidy
make generate     # go generate ./...
make outdated     # Show outdated direct dependencies
make godocs       # Open go docs

# Run a single test
go test ./commanderclient -run TestName
```

## Architecture

Single library package (`commanderclient/`) with a thin CLI entry point (`main.go`).

### Core abstractions

- **`Entity` interface** (`types.go`) — unified API over `EntryEntity` and `AssetEntity`. All field access is locale-aware with fallback support. Publishing status derived from version arithmetic: `draft` (PublishedVersion==0), `published` (Version-PublishedVersion==1), `changed` (>1).

- **`MigrationClient`** (`client.go`) — wraps the foomo/contentful CMA SDK. Optionally pairs a CDA client for published-view access. `LoadSpaceModel()` fetches locales, content types, entries, and assets concurrently into a `SpaceModel` cache. When a CDA key is provided, CDA views are loaded and attached to each entity (`entity.HasCDAView()`, `entity.CDAView()`). Set `Config.SkipAssets = true` to skip asset loading entirely. Provides entity lookups and filtering.

- **`EntityCollection`** (`collection.go`) — chainable operations on entity sets: filtering (50+ built-in filters), pagination, concurrent iteration (`ForEachConcurrent`), field extraction, grouping, stats, and conversion to migration operations.

- **`MigrationExecutor`** (`executor.go`) — runs `MigrationOperation` batches with configurable concurrency, dry-run mode, and per-operation confirmation. Update operations preserve publishing status.

### Translation subsystem

- **`DeepLTranslator`** (`translate.go`, `deepl.go`) — translates entity fields via DeepL API v2. Handles both simple strings and RichText (extracts text nodes, translates, reassembles). Tracks billed characters.
- **RichText processing** (`richtext_internal.go`, `hyperlinks.go`) — parses Contentful RichText JSON, extracts/replaces text nodes, processes hyperlinks.
- **Text utilities** (`textutils.go`) — `MatchCase`, `FixURI` (diacritics removal, slug generation), `ToLowerURL`.

### Configuration

Environment variables: `CONTENTFUL_CMAKEY`, `CONTENTFUL_CDAKEY` (optional, enables CDA views), `CONTENTFUL_SPACE_ID`, `CONTENTFUL_ENVIRONMENT`, `CONTENTFUL_VERBOSE`. Loaded via `LoadConfigFromEnv()` in `utils.go`. `Config.SkipAssets` is code-only (no env var).

## Conventions

- Local imports grouped under `github.com/foomo/contentfulcommander` (enforced by goimports)
- Tabs for indentation, trailing whitespace trimmed, LF line endings
- Build tag: `-tags=safe` used across build, install, and test targets
- `golangci-lint` v2 config in `.golangci.yml` — `default: all` with explicit disable list; formatters: gofmt, goimports
- Branch names must follow `feature/` or `fix/` prefix convention (enforced by lefthook pre-commit hook)
- Commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/) format: `type(scope?): subject` (enforced by lefthook commit-msg hook). Valid types: `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `style`, `test`, `sec`, `wip`, `revert`
