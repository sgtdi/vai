*The project was renamed from `watch` to `vai` to avoid a name conflict with the standard `watch` command on Linux systems.*

**`Vai` is the only hot reload system for Go that integrates its own file watcher directly, eliminating external dependencies. This streamlined, self-contained design means a shorter and clearer chain of responsibility, making bug tracking and resolution significantly more efficient.**

# vai: Hot reload Go apps and projects

[![Go Reference](https://pkg.go.dev/badge/github.com/sgtdi/vai.svg)](https://pkg.go.dev/github.com/sgtdi/vai)
[![Go Report Card](https://goreportcard.com/badge/github.com/sgtdi/vai)](https://goreportcard.com/report/github.com/sgtdi/vai)
[![CI](https://github.com/sgtdi/vai/actions/workflows/ci-test.yml/badge.svg)](https://github.com/sgtdi/vai/actions/workflows/ci-test.yml)
[![CodeQL](https://github.com/sgtdi/vai/actions/workflows/codeql.yml/badge.svg)](https://github.com/sgtdi/vai/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Automatic **hot reload** for **Go** development. Zero configuration CLI tool for Go developers seeking instant feedback and automated workflows.

`vai` is a lightweight, zero-dependency **CLI tool** that automatically rebuilds and restarts your Go applications when files change. Perfect for Go web development, microservices, REST APIs, and any Go project requiring rapid iteration.

Stop the tedious **cycle of manually stopping, rebuilding, and restarting** your project. `vai` automates this process, giving you instant feedback on every file change and commands execution. It's built for developers who value speed and simplicity, offering a **seamless, configuration-free experience** right out of the box.

## Index

- [Features](#features)
- [Why vai](#why-vai)
- [Installation](#installation)
- [Quick start](#quick-start)
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

## Why `vai`?

-   **Hot reload**: Seamlessly rebuilds and restarts your Go application the moment you save a file, keeping your development flow uninterrupte
-   **Workflow**: Start instantly with a single command, or orchestrate complex, multi-step tasks with an optional `vai.yml`
-   **No external dependencies:**: `vai` is a self-contained executable powered by our own [`fswatcher`](https://github.com/sgtdi/fswatcher) library for reliable, high-performance file monitoring
-   **Ready to use:** Ready by default for Go projects, allowing you to start hot-reloading from the CLI without writing a single line of configuration

## Installation

Ensure you have Go installed (version 1.24 or higher is recommended).

```sh
go install github.com/sgtdi/vai
```

## Quick start

The tool can be configured in two ways:

1.  **CLI Mode:** Provide flags and one or more commands directly
2.  **File Mode:** Use a `vai.yml` file for more complex workflows with multiple commands in series or parallel

### CLI mode

`vai` follows a simple syntax: `vai [flags] [commands]`.

#### Flags

| Flag &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; &nbsp; | Short | Description                                                               | Default                                                 |
|:-------------------------------------------------------------|:------|:--------------------------------------------------------------------------|:--------------------------------------------------------|
| `--cmd`                                                      | `-c`  | Command to run. Can be specified multiple times for sequential execution. | (none)                                                  |
| `--path`                                                     | `-p`  | Path to vai for changes.                                                | `.`                                                     |
| `--regex`                                                    | `-r`  | Comma-separated list of regex patterns for files to vai.                | `".*\\.go$", "^go\\.mod$", "^go\\.sum$"`                  |
| `--env`                                                      | `-e`  | Comma-separated list of `KEY=VALUE` pairs to set as environment variables.| (none)                                                  |
| `--file`                                                     | `-f`  | Load configuration from a YAML file instead of using CLI flags.           | `vai.yml`                                             |
| `--save`                                                     | `-s`  | Save the current CLI flags to a new YAML configuration file.              | (none)                                                  |
| `--debug`                                                    | `-d`  | Enable debug mode to print detailed configuration and event information.  | `false`                                                 |
| `--quiet`                                                    | `-q`  | Disable all logging output, showing only command results.                 | `false`                                                 |
| `--help`                                                     | `-h`  | Show the help message and exit.                                           | `false`                                                 |

### CLI use cases

Here are some practical examples of how to use `vai` from the command line.

**1. Basic Go hot reload**
The most common use case. `vai` will monitor all `.go` files, `go.mod`, and `go.sum` in the current directory and restart the application on any change.

```sh
vai go run .
```

**2. Automatically run tests**
Run all tests in your project whenever a Go file changes.

```sh
vai go test -v ./...
```

**3. Hot reload with environment variables**
Watch a specific directory (`./app`) and inject environment variables for a database connection.

```sh
vai --path=./app --env="DB_HOST=localhost,DB_PORT=5432" go run ./app
```

**4. Hot reload Go web server (e.g., Gin, Echo)**
Watch only `.go` files and `.html` templates to restart your web server.

```sh
vai --regex=".*\\.go$,.*\\.html$" go run .
```

**5. Save a command to a `vai.yml` file**
If a command gets too long, you can save it to a configuration file for easier reuse. This command creates a `dev.yml` file with the specified settings.

```sh
vai --path=./src --regex=".*\\.go$" --save dev.yml go run .
```
You can then run it with `vai --file dev.yml`.

**6. Chaining multiple commands**
Run a linter before executing your main application to catch errors early. `vai` will run the commands in sequence.

```sh
vai --cmd "golangci-lint run" --cmd "go run ."
```

### File mode (`vai.yml`)

For more complex workflows, you can create a `vai.yml` file to define multiple jobs with sequential and parallel steps.

**Example `vai.yml`:**

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

When you run `vai` in a directory with this `vai.yml`, it will:
- Run `go fmt` and then `go run .` sequentially when a `.go` file (that isn't a test file) changes
- Run `go test`, `go vet`, and `golangci-lint` all at the same time when a `_test.go` file changes

## Examples

For hands-on examples, check out the following directories. Each example includes a `vai.yml` and a [`README.md`](./examples/README.md) with detailed instructions.

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
