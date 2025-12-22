package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestJobUnmarshalYAML(t *testing.T) {
	t.Run("Unmarshal simple command string", func(t *testing.T) {
		yamlString := `"go run ."`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		expected := Job{Cmd: "go", Params: []string{"run", "."}}
		if !reflect.DeepEqual(job, expected) {
			t.Errorf("Expected %+v, got %+v", expected, job)
		}
	})

	t.Run("Unmarshal full job struct", func(t *testing.T) {
		yamlString := `
name: "Run and Test"
series:
  - cmd: "go build"
  - cmd: "./app"
env:
  APP_ENV: "development"
on:
  regex:
    - '\.go$'
`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if job.Name != "Run and Test" {
			t.Errorf("Expected name 'Run and Test', got '%s'", job.Name)
		}
		if len(job.Series) != 2 {
			t.Errorf("Expected 2 series jobs, got %d", len(job.Series))
		}
		if job.Env["APP_ENV"] != "development" {
			t.Errorf("Expected APP_ENV to be 'development', got '%s'", job.Env["APP_ENV"])
		}
		// The expected value in Go is an unescaped string.
		if len(job.On.Regex) != 1 || job.On.Regex[0] != `\.go$` {
			t.Errorf("Expected regex '\\.go$', got '%v'", job.On.Regex)
		}
	})

	t.Run("Unmarshal fails with multiple action types", func(t *testing.T) {
		yamlString := `
cmd: "do one thing"
series:
  - cmd: "do another thing"
`
		var job Job
		err := yaml.Unmarshal([]byte(yamlString), &job)
		if err == nil {
			t.Fatal("Expected an error but got none")
		}
	})
}

func TestFromCLI(t *testing.T) {
	t.Run("From single command", func(t *testing.T) {
		singleCmd := []string{"go", "run", "."}
		path := "./src"
		patterns := []string{`\.go$`}
		env := map[string]string{"PORT": "8080"}

		vai := FromCLI(nil, singleCmd, path, patterns, env)

		if vai.Config.Path != path {
			t.Errorf("Expected path '%s', got '%s'", path, vai.Config.Path)
		}

		job := vai.Jobs["default"]
		if len(job.Series) != 1 {
			t.Fatalf("Expected 1 series job, got %d", len(job.Series))
		}
		if job.Series[0].Cmd != "go" || !reflect.DeepEqual(job.Series[0].Params, []string{"run", "."}) {
			t.Errorf("Unexpected command: %+v", job.Series[0])
		}
		if job.Env["PORT"] != "8080" {
			t.Errorf("Expected env PORT=8080, got '%s'", job.Env["PORT"])
		}
		if !reflect.DeepEqual(job.On.Regex, patterns) {
			t.Errorf("Expected regex patterns '%v', got '%v'", patterns, job.On.Regex)
		}
	})

	t.Run("From multiple cmd flags", func(t *testing.T) {
		seriesCmds := []string{"go fmt ./...", "go run ."}
		vai := FromCLI(seriesCmds, nil, ".", nil, nil)

		job := vai.Jobs["default"]
		if len(job.Series) != 2 {
			t.Fatalf("Expected 2 series jobs, got %d", len(job.Series))
		}
		if job.Series[0].Cmd != "go" || !reflect.DeepEqual(job.Series[0].Params, []string{"fmt", "./..."}) {
			t.Errorf("Unexpected first command: %+v", job.Series[0])
		}
		if job.Series[1].Cmd != "go" || !reflect.DeepEqual(job.Series[1].Params, []string{"run", "."}) {
			t.Errorf("Unexpected second command: %+v", job.Series[1])
		}
	})
}

func TestFromFile(t *testing.T) {
	t.Run("Successfully load a valid file", func(t *testing.T) {
		yamlContent := `
config:
  path: "/app"
jobs:
  default:
    series:
      - "go run ."
`
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "vai.yml")
		os.WriteFile(filePath, []byte(yamlContent), 0644)

		vai, err := FromFile(filePath, "", false)
		if err != nil {
			t.Fatalf("FromFile failed: %v", err)
		}

		if vai.Config.Path != "/app" {
			t.Errorf("Expected path '/app', got '%s'", vai.Config.Path)
		}
		if _, ok := vai.Jobs["default"]; !ok {
			t.Error("Expected 'default' job to be present")
		}
	})

	t.Run("Path flag overrides file path", func(t *testing.T) {
		yamlContent := `
config:
  path: "/app"
`
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "vai.yml")
		os.WriteFile(filePath, []byte(yamlContent), 0644)

		vai, err := FromFile(filePath, "/override", true)
		if err != nil {
			t.Fatalf("FromFile failed: %v", err)
		}

		if vai.Config.Path != "/override" {
			t.Errorf("Expected path to be overridden to '/override', but got '%s'", vai.Config.Path)
		}
	})

	t.Run("Return error for non-existent file", func(t *testing.T) {
		_, err := FromFile("non-existent-file.yml", "", false)
		if err == nil {
			t.Fatal("Expected an error for a non-existent file, but got none")
		}
	})

	t.Run("Return error for malformed YAML", func(t *testing.T) {
		yamlContent := `config: { path: "/app }`
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "vai.yml")
		os.WriteFile(filePath, []byte(yamlContent), 0644)

		_, err := FromFile(filePath, "", false)
		if err == nil {
			t.Fatal("Expected an error for malformed YAML, but got none")
		}
	})
}
