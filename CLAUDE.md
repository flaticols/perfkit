# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**perfkit** is a Go CLI tool. The project uses SQLite for data storage (via modernc.org/sqlite - pure Go, no CGO), go-flags for CLI argument parsing, and goqu for SQL query building.

## Build & Run Commands

```bash
# Build the CLI
go build -o perfkit ./cmd/perfkit

# Run directly
go run ./cmd/perfkit

# Run tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a single test
go test -v -run TestName ./path/to/package
```

## Project Structure

```
cmd/perfkit/     # CLI entry point
```

## Key Dependencies

- `modernc.org/sqlite` - Pure Go SQLite (no CGO required)
- `github.com/jessevdk/go-flags` - CLI argument parsing
- `github.com/doug-martin/goqu/v9` - SQL query builder
- `github.com/jmoiron/sqlx` - SQL extensions for database/sql

## Issue Tracking

This project uses Beads (`bd` CLI) for issue tracking. Issues are stored in `.beads/` and sync with git.

```bash
bd ready              # Show issues ready to work
bd create --title="..." --type=task  # Create new issue
bd update <id> --status=in_progress  # Start work
bd close <id>         # Complete issue
bd sync --from-main   # Sync beads from main branch
```
