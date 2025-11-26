# watch: Hot reload Go apps and projects

[![Go Reference](https://pkg.go.dev/badge/github.com/sgtdi/watch.svg)](https://pkg.go.dev/github.com/sgtdi/watch)
[![Go Report Card](https://goreportcard.com/badge/github.com/sgtdi/watch)](https://goreportcard.com/report/github.com/sgtdi/watch)
[![CI](https://github.com/sgtdi/watch/actions/workflows/ci-test.yml/badge.svg)](https://github.com/sgtdi/watch/actions/workflows/ci-test.yml)
[![CodeQL](https://github.com/sgtdi/watch/actions/workflows/codeql.yml/badge.svg)](https://github.com/sgtdi/watch/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Automatic **hot reload** for **Go** development. Zero configuration CLI tool for Go developers seeking instant feedback and automated workflows.

`watch` is a lightweight, zero-dependency **CLI tool** that automatically rebuilds and restarts your Go applications when files change. Perfect for Go web development, microservices, REST APIs, and any Go project requiring rapid iteration.

Stop the tedious **cycle of manually stopping, rebuilding, and restarting** your project. `watch` automates this process, giving you instant feedback on every file change and commands execution. It's built for developers who value speed and simplicity, offering a **seamless, configuration-free experience** right out of the box.

## Index

- [Features](#features)
- [Why Choose watch](#why-watch)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)

## Features

- 🔥 Hot reload: Automatically detects [README.md](README.md)file changes and restarts your Go application instantly
- ⚡  Zero configuration: Works out-of-the-box for Go projects, no setup required
- 🎯 Zero external deps: Self contained executable using [fswatcher](https://github.com/sgtdi/fswatcher) for high-performance file monitoring
- 🔧 Flexible workflows: Simple CLI mode for quick tasks, YAML configuration for complex multi-step workflows
- 🚀 Production ready: Built for Go 1.21+, optimized for Go web frameworks and advanced pipelines
- 📝 Smart file watching: Regex pattern matching, exclusion rules, and directory-specific monitoring
- ⚙️ Environment vars: Easy injection of environment variables for different development scenarios
- 🔄 Sequential & Parallel Execution: Run multiple commands in series or parallel for comprehensive workflows

## Why `watch`?

-   **Hot reload**: Seamlessly rebuilds and restarts your Go application the moment you save a file, keeping your development flow uninterrupte
-   **Workflow**: Start instantly with a single command, or orchestrate complex, multi-step tasks with an optional `watch.yml`
-   **No external dependencies:**: `watch` is a self-contained executable powered by our own [`fswatcher`](https://github.com/sgtdi/fswatcher) library for reliable, high-performance file monitoring
-   **Ready to use:** Ready by default for Go projects, allowing you to start hot-reloading from the CLI without writing a single line of configuration

## Installation

Ensure you have Go installed (version 1.24 or higher is recommended).

```sh
go install github.com/sgtdi/watch
```

## Quick start

The tool can be configured in two ways:

1.  **CLI Mode:** Provide flags and one or more commands directly
2.  **File Mode:** Use a `watch.yml` file for more complex workflows with multiple commands in series or parallel

### CLI mode

`watch` follows a simple syntax: `watch [flags] [commands]`.

#### Flags

| Flag &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; | Short | Description                                                               | Default                                                 |
|:-------------------------------------------------------------|:------|:--------------------------------------------------------------------------|:--------------------------------------------------------|
| `--cmd`                                                      | `-c`  | Command to run. Can be specified multiple times for sequential execution. | (none)                                                  |
| `--path`                                                     | `-p`  | Path to watch for changes.                                                | `.`                                                     |
| `--regex`                                                    | `-r`  | Comma-separated list of regex patterns for files to watch.                | `".*\\.go$", "^go\\.mod$", "^go\\.sum$"`                  |
| `--env`                                                      | `-e`  | Comma-separated list of `KEY=VALUE` pairs to set as environment variables.| (none)                                                  |
| `--file`                                                     | `-f`  | Load configuration from a YAML file instead of using CLI flags.           | `watch.yml`                                             |
| `--save`                                                     | `-s`  | Save the current CLI flags to a new YAML configuration file.              | (none)                                                  |
| `--debug`                                                    | `-d`  | Enable debug mode to print detailed configuration and event information.  | `false`                                                 |
| `--quiet`                                                    | `-q`  | Disable all logging output, showing only command results.                 | `false`                                                 |
| `--help`                                                     | `-h`  | Show the help message and exit.                                           | `false`                                                 |

### CLI use cases

Here are some practical examples of how to use `watch` from the command line.

**1. Basic Go hot reload**
The most common use case. `watch` will monitor all `.go` files, `go.mod`, and `go.sum` in the current directory and restart the application on any change.

```sh
watch go run .
```

**2. Automatically run tests**
Run all tests in your project whenever a Go file changes.

```sh
watch go test -v ./...
```

**3. Hot reload with environment variables**
Watch a specific directory (`./app`) and inject environment variables for a database connection.

```sh
watch --path=./app --env="DB_HOST=localhost,DB_PORT=5432" go run ./app
```

**4. Hot reload Go web server (e.g., Gin, Echo)**
Watch only `.go` files and `.html` templates to restart your web server.

```sh
watch --regex=".*\\.go$,.*\\.html$" go run .
```

**5. Save a command to a `watch.yml` file**
If a command gets too long, you can save it to a configuration file for easier reuse. This command creates a `dev.yml` file with the specified settings.

```sh
watch --path=./src --regex=".*\\.go$" --save dev.yml go run .
```
You can then run it with `watch --file dev.yml`.

**6. Chaining multiple commands**
Run a linter before executing your main application to catch errors early. `watch` will run the commands in sequence.

```sh
watch --cmd "golangci-lint run" --cmd "go run ."
```

### File mode (`watch.yml`)

For more complex workflows, you can create a `watch.yml` file to define multiple jobs with sequential and parallel steps.

**Example `watch.yml`:**

```yaml
config:
  path: .
  cooldown: 100ms
  logLevel: info

jobs:
  # This job runs the main application on changes to Go files
  run-app:
    on:
      regex:
        - ".*\\.go$"
        - "!.*_test.go$" # Exclude test files
    series:
      - cmd: "go fmt ./..."
      - cmd: "go run ."

  # This job runs tests and linters in parallel on changes to test files
  run-quality-checks:
    on:
      regex:
        - ".*_test\\.go$"
    parallel:
      - cmd: "go test -v ./..."
      - cmd: "go vet ./..."
      - cmd: "golangci-lint run"
```

When you run `watch` in a directory with this `watch.yml`, it will:
- Run `go fmt` and then `go run .` sequentially when a `.go` file (that isn't a test file) changes
- Run `go test`, `go vet`, and `golangci-lint` all at the same time when a `_test.go` file changes

## Examples

For hands-on examples, check out the following directories. Each example includes a `watch.yml` and a [`README.md`](./examples/README.md) with detailed instructions.

| Example                                                   | Description                                                               |
|:----------------------------------------------------------|:--------------------------------------------------------------------------|
| [`simple-test`](./examples/simple-test)                    | Run tests on file changes       |
| [`build-and-run`](./examples/build-and-run)                | Build a binary and run it |
| [`web-server`](./examples/web-server)                      | Hot reloading a Go web server                     |
| [`advanced-workflow`](./examples/advanced-workflow)        | Complex workflow with multiple jobs |

## Contributing

Contributions are welcome! Please feel free to open an issue or submit a pull request.

## License

This project is licensed under the MIT License.
