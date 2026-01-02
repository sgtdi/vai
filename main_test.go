package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"
)

func init() {
	logger = newLogger(SeverityError)
}

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	f()

	w.Close()
	os.Stdout = old
	return <-outC
}

func TestNewVai(t *testing.T) {
	t.Run("CLI arguments take precedence", func(t *testing.T) {
		cmdFlags := []string{"go run ."}
		cli := &Args{
			CmdFlags:   cmdFlags,
			Path:       "./app",
			ConfigFile: "vai.yml",
		}
		vai, err := newVai(cli)
		if err != nil {
			t.Fatalf("newVai failed: %v", err)
		}

		// Check default job paths instead of Config.Path
		defaultJob, ok := vai.Jobs["default"]
		if !ok {
			t.Error("Expected a 'default' job to be created from CLI args")
		}
		if len(defaultJob.Trigger.Paths) != 1 || defaultJob.Trigger.Paths[0] != "./app" {
			t.Errorf("Expected default job path to be './app', got '%v'", defaultJob.Trigger.Paths)
		}
	})

	t.Run("Fallback to file when no CLI commands", func(t *testing.T) {
		content := `
jobs:
  from-file:
    cmd: echo hello
    trigger:
      paths: ["/file"]
`
		tmpfile, err := os.CreateTemp("", "vai.*.yml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.Write([]byte(content))
		tmpfile.Close()

		cli := &Args{
			ConfigFile: tmpfile.Name(),
		}
		vai, err := newVai(cli)
		if err != nil {
			t.Fatalf("newVai failed: %v", err)
		}

		// Check job path from file
		job, ok := vai.Jobs["from-file"]
		if !ok {
			t.Error("Expected job 'from-file' to be loaded from the config file")
		}
		if len(job.Trigger.Paths) != 1 || job.Trigger.Paths[0] != "/file" {
			t.Errorf("Expected job path to be '/file', got '%v'", job.Trigger.Paths)
		}
	})

	t.Run("Debug severity is set correctly via config", func(t *testing.T) {
		content := `
config:
  severity: debug
`
		tmpfile, err := os.CreateTemp("", "vai.*.yaml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.Write([]byte(content))
		tmpfile.Close()

		cli := &Args{
			ConfigFile: tmpfile.Name(),
		}
		vai, err := newVai(cli)
		if err != nil {
			t.Fatalf("newVai failed: %v", err)
		}

		if vai.Config.Severity != SeverityDebug.String() {
			t.Errorf("Expected Severity to be 'debug', got '%s'", vai.Config.Severity)
		}
	})
}

func TestParseArgs(t *testing.T) {
	t.Run("parses boolean flags", func(t *testing.T) {
		args := []string{"--debug", "--help", "--version", "--save"}
		cli := parseArgs(args)

		if !cli.Debug {
			t.Error("Expected Debug to be true")
		}
		if !cli.Help {
			t.Error("Expected Help to be true")
		}
		if !cli.Version {
			t.Error("Expected Version to be true")
		}
		if !cli.Save {
			t.Error("Expected Save to be true")
		}
	})

	t.Run("parses value flags", func(t *testing.T) {
		args := []string{"--path", "./foo", "--env", "K=V", "--regex", ".*"}
		cli := parseArgs(args)

		if cli.Path != "./foo" {
			t.Errorf("Expected Path to be './foo', got '%s'", cli.Path)
		}
		if cli.Env != "K=V" {
			t.Errorf("Expected Env to be 'K=V', got '%s'", cli.Env)
		}
		if cli.Regex != ".*" {
			t.Errorf("Expected Regex to be '.*', got '%s'", cli.Regex)
		}
	})

	t.Run("parses short flags", func(t *testing.T) {
		args := []string{"-p", "./bar", "-e", "A=B", "-r", "^main", "-d", "-s"}
		cli := parseArgs(args)

		if cli.Path != "./bar" {
			t.Errorf("Expected Path to be './bar', got '%s'", cli.Path)
		}
		if cli.Env != "A=B" {
			t.Errorf("Expected Env to be 'A=B', got '%s'", cli.Env)
		}
		if cli.Regex != "^main" {
			t.Errorf("Expected Regex to be '^main', got '%s'", cli.Regex)
		}
		if !cli.Debug {
			t.Error("Expected Debug to be true")
		}
		if !cli.Save {
			t.Error("Expected Save to be true")
		}
	})

	t.Run("parses command flags", func(t *testing.T) {
		args := []string{"--cmd", "echo hello", "-c", "ls -la"}
		cli := parseArgs(args)

		expectedCmds := []string{"echo hello", "ls -la"}
		if !reflect.DeepEqual(cli.CmdFlags, expectedCmds) {
			t.Errorf("Expected CmdFlags to be %v, got %v", expectedCmds, cli.CmdFlags)
		}
	})

	t.Run("parses positional arguments", func(t *testing.T) {
		args := []string{"-p", ".", "echo", "arg1", "arg2"}
		cli := parseArgs(args)

		if cli.Path != "." {
			t.Errorf("Expected Path to be '.', got '%s'", cli.Path)
		}
		expectedPositional := []string{"echo", "arg1", "arg2"}
		if !reflect.DeepEqual(cli.PositionalArgs, expectedPositional) {
			t.Errorf("Expected PositionalArgs to be %v, got %v", expectedPositional, cli.PositionalArgs)
		}
	})

	t.Run("parses command flag consuming subsequent args", func(t *testing.T) {
		// This tests the parseCmdFlag logic where it consumes args until next flag
		args := []string{"--cmd", "go", "run", ".", "--debug"}
		cli := parseArgs(args)

		expectedCmds := []string{"go run ."}
		if !reflect.DeepEqual(cli.CmdFlags, expectedCmds) {
			t.Errorf("Expected CmdFlags to be %v, got %v", expectedCmds, cli.CmdFlags)
		}
		if !cli.Debug {
			t.Error("Expected Debug to be true")
		}
	})

	t.Run("parses flags with attached values", func(t *testing.T) {
		args := []string{"--path=./baz", "--env=X=Y"}
		cli := parseArgs(args)

		if cli.Path != "./baz" {
			t.Errorf("Expected Path to be './baz', got '%s'", cli.Path)
		}
		if cli.Env != "X=Y" {
			t.Errorf("Expected Env to be 'X=Y', got '%s'", cli.Env)
		}
	})
}
