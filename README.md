# gorep
[![Tests](https://github.com/vinzenta/gorep/actions/workflows/test.yml/badge.svg)](https://github.com/vinzenta/gorep/actions/workflows/test.yml)
![Test Coverage](https://img.shields.io/badge/coverage-92.9%25-brightgreen)
![Go Version](https://img.shields.io/badge/go-1.24-blue)


A small, fast grep-like tool written in Go. `gorep` searches text for a regular expression and prints matching lines with colored highlights. It supports reading from stdin, searching an inline string, a file, or recursively searching files in a directory.

**Key Features**
- Pattern search using Go regular expressions (`regexp`).
- Colored output: matches highlighted in green, file/line labels in red/white (via `fortio.org/terminal/ansipixels/tcolor`).
- Supports reading from `stdin`, a single file, or a directory (recurses into subdirectories).
- `-no-trim` option to preserve leading whitespace in printed lines.

Requirements
- Go (1.18+ recommended). Verify with `go version`.

Build

```powershell
go mod tidy
go build -o gorep .
```

Run (examples)

- Search piped input:

```powershell
echo "hello world" | gorep "world"
```

- Search an inline string (pattern then text):

```powershell
gorep "foo" "this is foo and that is bar"
```

- Search a file:

```powershell
gorep -f README.md "TODO"
```

- Search a directory (recursively):

```powershell
gorep -f ./src "TODO"
```

Flags
- `-f <path>` : read input from a file or directory. If a directory is provided, `gorep` will walk the directory and search files it can read concurrently.
- `-no-trim` : disable trimming leading indentation in each printed line. By default `gorep` trims leading tabs/spaces around matches.
- `-o <path>` : path to output file, where gorep will write each match.
- `-workers <n>` : number of concurrent workers for directory search (default: number of CPU cores)

[NOTE] Flags must come BEFORE the pattern argument (standard Go flag package behavior).

Behavior details
- Flags must be provided BEFORE the pattern argument (standard Go convention).
- The first non-flag argument is treated as the regular expression pattern. If it includes a space, it should be inside quotation marks "<pattern>".
- If additional non-flag arguments are provided after the pattern they are joined into a single input string to search (convenient for one-off searches from the CLI).
- If no `-f` is provided and no inline text is given, `gorep` reads from `stdin` until EOF.
- Matches in a line are highlighted in green; printed lines are numbered and prefixed with color-coded labels. When searching directories, each file's results are prefixed by the filename.
- Directory searches use concurrent workers (configurable with `-workers`) for improved performance on multi-core systems.

Improvements in This Version
- [CONCURRENT] Multi-threaded file processing with worker pool pattern (default: CPU cores)
- [STREAMING] Files are processed as discovered, not loaded all into memory
- [OPTIMIZED] Regex runs once per line (not twice) and efficient string building
- [ROBUST] Proper error handling with context cancellation support
- [SAFE] Uses absolute paths, never changes working directory

Limitations & Notes
- Files that cannot be read or are not valid UTF-8 are skipped silently when walking directories.
- Errors (invalid regexp, unreadable file, etc.) will log a message and exit with a non-zero status.

Testing

```powershell
go test ./...
```

Contributing
- Open issues or pull requests. Small, focused changes are easiest to review.

Source
- Main implementation: `main.go`
