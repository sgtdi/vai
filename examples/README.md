# Watch: Usage Examples

This directory contains several examples to demonstrate the capabilities of the `watch` tool, from simple hot-reloading to complex, multi-job workflows

---

### 1. `simple-test`

**Purpose:** Demonstrates how to use `watch` to automatically run Go tests and inject environment variables

-   **`watch.yml`:** Configured to run `go test -v ./...` on any change to a `.go` file. It also sets the `TEST_MODE` and `DB_HOST` environment variables, which are checked in the test file
-   **`main_test.go`:** Contains a simple unit test and an "integration" test that only runs if the correct environment variables are set

---

### 2. `build-and-run`

**Purpose:** Shows a sequential, two-step workflow: building a Go binary and then executing it

-   **`watch.yml`:** Uses the `series` keyword to first run `go build -o app .` and, upon success, run the compiled `./app` binary
-   **`main.go`:** A simple application that prints a timestamp to confirm it was built and run

---

### 3. `web-server`

**Purpose:** A classic hot-reload example for a Go web server

-   **`watch.yml`:** A minimal configuration that runs `go run .` on any change to a `.go` file
-   **`main.go`:** A basic web server that listens on port `:8080`. It's designed to show the effect of hot-reloading clearly

---

### 4. `advanced-workflow`

**Purpose:** A more complex, realistic example demonstrating multi-job workflows, parallel execution, and different triggers for application code versus tests

-   **`watch.yml`:**
    -   A `run-dev-server` job that triggers on changes to application (`.go`) files
    -   A `run-tests` job that triggers only on changes to test (`_test.go`) files and runs tests and a linter in parallel
    -   A `build` job that is only run manually (e.g., `watch build`)
-   **`main.go`, `handlers.go`, `main_test.go`:** A multi-file web service that reads environment variables, making it a good showcase for the different job configurations
