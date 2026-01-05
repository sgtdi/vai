[![Go Reference](https://pkg.go.dev/badge/github.com/sgtdi/vai.svg)](https://pkg.go.dev/github.com/sgtdi/vai)
[![Go Report Card](https://goreportcard.com/badge/github.com/sgtdi/vai)](https://goreportcard.com/report/github.com/sgtdi/vai)
[![CI](https://github.com/sgtdi/vai/actions/workflows/ci-test.yml/badge.svg)](https://github.com/sgtdi/vai/actions/workflows/ci-test.yml)
[![CodeQL](https://github.com/sgtdi/vai/actions/workflows/codeql.yml/badge.svg)](https://github.com/sgtdi/vai/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

# Vai: Hot reload Go apps and projects

> *Vai (Italian: "Go!") - Because your code should go as fast as you do*

**Stop the endless cycle of Ctrl+C → rebuild → restart** 

Vai automatically rebuilds and restarts your Go applications the instant you save a file. Zero configuration, hassle and third-party dependencies.

<img src="https://raw.githubusercontent.com/sgtdi/watch/refs/heads/dev/examples/vai-go-hot-reload.png" width="100%" alt="vai golang hot reload terminal interface">

## Index

- [Get started](#-get-started-in-5-seconds)
- [Why Vai?](#-why-vai)
- [Use cases](#-use-cases)
- [CLI reference](#-cli-reference)
- [Advanced configuration](#-advanced-configuration-using-vaiyml)
- [Real examples](#-real-examples)
- [How it works](#-how-it-works)
- [Tips and tricks](#-tips-and-tricks)
- [Migrating](#-migrating-from-other-tools)
- [Troubleshooting](#-troubleshooting)
- [Contributing](#contributing)

## ⚡ Get started in 5 seconds

```bash
# Install
go install github.com/sgtdi/vai@latest

# Run
vai go run .

#Your app now hot reloads on every change
```

No YAML files required, no configuration or external dependencies. **It just works**

## 🎯 Why Vai?

| Feature | Vai | Air | Fresh | Realize |
|---------|:---:|:---:|:-----:|:-------:|
| Works out of the box | ✅ | ✅  | ❌ | ❌ |
| Zero config/flags needed | ✅ | ❌ | ❌ | ❌ |
| No external dependencies | ✅ | ❌ | ❌ | ❌ |
| CLI-first design | ✅ | ✅ | ✅ | ❌ |
| Parallel job execution | ✅ | ❌ | ❌ | ✅ |
| Built-in file watcher | ✅ | ❌ | ❌ | ❌ |
| Setup time | 5 sec | 1 min | 3 min | 5 min |

**Vai is the only hot reload tool for Go that:**
- Requires **zero configuration** to get started
- Has **zero external dependencies** (self-contained with built-in [fswatcher](https://github.com/sgtdi/fswatcher))
- Works **instantly** - from install to hot reload in 5 seconds
- Supports **complex workflows** with parallel and sequential job execution (when you need it)

## 🔥 Use Cases

### Web development (Gin, Echo, Chi, Fiber)

```bash
vai go run .
```

Your web server restarts instantly every time you save, perfect for api development.

### Test-driven development

```bash
vai go test -v ./...
```

Tests run automatically on every change. See results immediately.

### Environment variables

```bash
vai --env="PORT=8080,DB_HOST=localhost,DB_USER=admin" go run .
```

Inject environment variables without shell scripts.

### Watch specific files

```bash
# Only watch .go and .html files
vai --regex=".*\\.go$,.*\\.html$" go run ./cmd/server

# Ignore test files
vai --regex=".*\\.go$,!.*_test.go$" go run .
```

Fine-grained control over what triggers rebuilds.

### Multiple commands (Chaining)

```bash
# Format then run
vai --cmd "go fmt ./..." --cmd "go run ."

# Lint, test, then run
vai --cmd "golangci-lint run" --cmd "go test ./..." --cmd "go run ."
```

Chain commands together and run in sequence.

### Save and customize your configuration

```bash
# Save commands to vai.yml
vai --path=./app --regex=".*\\.go$" --env="PORT=8080" --save go run .

# Then just run
vai
```

## 📖 CLI reference

```
vai [flags] [command]

USAGE:
  vai go run .                    # Simple hot reload
  vai --cmd "cmd1" --cmd "cmd2"   # Multiple commands
  vai                             # Use vai.yml config

FLAGS:
  -c, --cmd string      Command to run (can be used multiple times for sequential execution)
  -p, --path string     Path to watch for changes (default: ".")
  -r, --regex string    Comma-separated regex patterns for files to watch (default: ".*\\.go$,^go\\.mod$,^go\\.sum$")
  -e, --env string      Comma-separated KEY=VALUE pairs for environment variables
  -s, --save string     Save current CLI flags to a YAML configuration file
  -d, --debug           Enable debug mode with detailed output and create a debug.log to record watcher events
  -h, --help            Show this help message

EXAMPLES:
  # Basic hot reload
  vai go run .
  
  # Watch specific directory
  vai --path=./cmd/api go run ./cmd/api
  
  # Custom file patterns
  vai --regex=".*\\.go$,.*\\.html$,.*\\.css$" go run .
  
  # With environment variables
  vai --env="DEBUG=true,PORT=3000" go run .
  
  # Chain multiple commands
  vai --cmd "go generate ./..." --cmd "go run ."
  
  # Save configuration to vai.yml
  vai --path=./app --env="ENV=dev" --save go run ./app
```

## 🔧 Advanced configuration using `vai.yml`

For complex projects with multiple workflows, you can create a `vai.yml` file, that will be automatically detect and used.

### Simple config

```yaml
config:
  clearCli: true  # Clear screen before each reload

jobs:
  # Main application
  run-app:
    trigger:
      regex:
        - ".*\\.go$"
        - "!.*_test.go$"  # Exclude test files
    series:
      - cmd: "go fmt ./..."
      - cmd: "go run ."

  # Run tests when test files change
  test:
    trigger:
      paths:
        - . # Entrypoint to watch for changes, multiple paths can be specified
      regex:
        - ".*_test\\.go$"
    series:
      - cmd: "go test -v ./..."
```

### Parallel jobs

```yaml
config:
  severity: info # Set logging level: debug, info, warn, error (default: warn)
  clearCli: true
  cooldown: 200ms

jobs:
  # Development server
  dev-server:
    trigger:
      regex:
        - ".*\\.go$"
        - "!.*_test.go$"
        - ".*\\.html$"
        - ".*\\.css$"
    series:
      - cmd: "go fmt ./..."
      - cmd: "go run ./cmd/server"
    env:
      - "ENV=development"
      - "PORT=8080"

  # Quality checks (runs in parallel)
  quality:
    trigger:
      regex:
        - ".*_test\\.go$"
    parallel:  # All commands run simultaneously
      - cmd: "go test -v -race ./..."
      - cmd: "go vet ./..."
      - cmd: "golangci-lint run --fast"
      - cmd: "staticcheck ./..."

  # Assets pipeline
  assets:
    trigger:
      regex:
        - ".*\\.scss$"
        - ".*\\.js$"
    series:
      - cmd: "npm run build:css"
      - cmd: "npm run build:js"
```

### CLI and watcher customization

```yaml
config:
  severity: warn             # Logging level: debug, info, warn, error (default: warn)
  clearCli: false            # Clear terminal before running jobs (default: false)
  cooldown: 100ms            # Wait time after file change to prevent duplicate triggers
  batchingDuration: 1s       # Group multiple rapid changes into single trigger
  bufferSize: 4096           # Event buffer size for high-velocity changes
```

## 📚 Real examples

Complete working examples are in the [`examples/`](examples/) directory:

| Example | Description | Use Case |
|---------|-------------|----------|
| [**simple-test**](examples/simple-test) | Auto-run tests on file changes | TDD workflow |
| [**build-and-run**](examples/build-and-run) | Build binary then execute | Production-like workflow |
| [**web-server**](examples/web-server) | Hot reload web application | Web development |
| [**advanced-workflow**](examples/advanced-workflow) | Parallel jobs & complex pipelines | Enterprise projects |

Each example includes:
- Complete `vai.yml` configuration
- Sample Go application
- Detailed README with instructions

## 🎓 How it works

Vai uses a custom-built file watcher called [fswatcher](https://github.com/sgtdi/fswatcher) that monitors your project for changes. When a file matching your patterns is modified:

1. **Event Detection**: fswatcher detects the file change
2. **Debouncing**: Vai waits for the cooldown period (default 100ms) to batch rapid changes
3. **Job Matching**: Finds all jobs with regex patterns matching the changed file
4. **Execution**: Runs matched jobs (series = sequential, parallel = simultaneous)
5. **Process Management**: Cleanly stops the old process and starts the new one

This architecture means:
- ⚡ **Fast**: No external process spawning overhead
- 🎯 **Reliable**: Purpose-built file watcher with proven stability
- 🔒 **Safe**: Proper process cleanup prevents zombie processes
- 📦 **Simple**: Everything in one binary

## 💡 Tips and tricks

### Prevent duplicate rebuilds

```yaml
config:
  cooldown: 200ms           # Wait 200ms after last change
  batchingDuration: 500ms   # Group changes within 500ms window
```

Useful when your editor saves multiple files simultaneously.

### Watch specific directories

```bash
vai --path=./internal go run ./cmd/api
```

Ignore changes outside your main source directory.

### Debugging file watcher and general issues

```bash
vai --debug go run .
```

Shows exactly which files are being watched and which events trigger rebuilds. 

### Exclude Generated Files

```yaml
jobs:
  app:
    trigger:
      regex:
        - ".*\\.go$"
        - "!.*\\.pb\\.go$"      # Exclude protobuf generated files
        - "!.*_gen\\.go$"       # Exclude go:generate output
        - "!vendor/.*"          # Exclude vendor directory
```

### Multiple envs

```bash
# Development
vai --env="ENV=dev,DB_HOST=localhost" go run .

# Staging (save to vai.staging.yml)
vai --env="ENV=staging,DB_HOST=staging.db" --save=vai.staging.yml go run .

# Use staging config
vai -f vai.staging.yml
```

## 🔄 Migrating from other tools

### From Air

**Air** requires `air.toml`:
```toml
[build]
  cmd = "go build -o ./tmp/main ."
  bin = "./tmp/main"
  include_ext = ["go", "html"]
```

**Vai** needs nothing:
```bash
vai go run .
```

Or with equivalent config:
```yaml
jobs:
  app:
    trigger:
      regex: [".*\\.go$", ".*\\.html$"]
    series:
      - cmd: "go run ."
```

### From Fresh

**Fresh** requires `runner.conf`:
```
root:              .
tmp_path:          ./tmp
build_name:        runner-build
build_log:         runner-build-errors.log
```

**Vai** needs nothing:
```bash
vai go run .
```

## 🐛 Troubleshooting

### Vai doesn't detect file changes

Check your regex patterns:
   ```bash
   vai --debug go run .  # Shows which files are watched
   ```

Increase buffer size for large projects:
   ```yaml
   config:
     bufferSize: 8192
   ```

Some editors save via rename/delete, just adjust cooldown:
   ```yaml
   config:
     cooldown: 300ms
   ```

### Process doesn't stop cleanly

Vai sends SIGTERM (Unix) or taskkill (Windows) to processes. If your app doesn't handle shutdown gracefully:

```go
// Add signal handling to your main.go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()
    
    // Your application code
    
    <-ctx.Done()
    // Cleanup
}
```

### High CPU usage

Reduce file system events:

```yaml
config:
  cooldown: 500ms           # Higher cooldown
  batchingDuration: 1s      # Batch events
jobs:
  app:
    trigger:
      regex:
        - "internal/.*\\.go$"  # Watch only specific dirs
        - "!vendor/.*"         # Exclude vendor
        - "!node_modules/.*"   # Exclude node_modules
```

## Contributing

Contributions are welcome! Help making Vai better.

- 🐛 **Report bugs** - [Open an issue](https://github.com/sgtdi/vai/issues/new)
- 💡 **Suggest features** - [Start a discussion](https://github.com/sgtdi/vai/discussions/new)
- 📖 **Improve docs** - Submit a PR with documentation fixes
- 🔧 **Submit PRs** - Check [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines
- ⭐ **Star the repo** - Helps others discover Vai
