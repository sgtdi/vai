package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"
)

func init() {
	logger = New(SeverityError)
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
		// NewVai(cmdFlags, positionalArgs, path, regex, env, configFile, help, severity)
		vai := NewVai(cmdFlags, nil, "./app", "", "", "vai.yml", false, SeverityWarn)

		if vai.Config.Path != "./app" {
			t.Errorf("Expected path to be './app', got '%s'", vai.Config.Path)
		}
		if _, ok := vai.Jobs["default"]; !ok {
			t.Error("Expected a 'default' job to be created from CLI args")
		}
	})

	t.Run("Fallback to file when no CLI commands", func(t *testing.T) {
		content := `
config:
  path: /file
jobs:
  from-file:
    cmd: echo hello
`
		tmpfile, err := os.CreateTemp("", "vai.*.yml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.Write([]byte(content))
		tmpfile.Close()

		vai := NewVai(nil, nil, "", "", "", tmpfile.Name(), false, SeverityWarn)

		if vai.Config.Path != "/file" {
			t.Errorf("Expected path to be '/file', got '%s'", vai.Config.Path)
		}
		if _, ok := vai.Jobs["from-file"]; !ok {
			t.Error("Expected job 'from-file' to be loaded from the config file")
		}
	})

	t.Run("Debug severity is set correctly", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "vai.*.yaml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		vai := NewVai(nil, nil, "", "", "", tmpfile.Name(), false, SeverityDebug)
		if vai.Config.Severity != SeverityDebug.String() {
			t.Errorf("Expected Severity to be 'debug', got '%s'", vai.Config.Severity)
		}
	})
}

func TestHandleCmds(t *testing.T) {
	v := &Vai{}

	t.Run("cmd flags take precedence", func(t *testing.T) {
		cmdFlags := []string{"cmd1", "cmd2"}
		positional := []string{"pos1", "pos2"}
		series, single := v.handleCmds(cmdFlags, positional)

		if !reflect.DeepEqual(series, cmdFlags) {
			t.Errorf("Expected series to be %v, got %v", cmdFlags, series)
		}
		if single != nil {
			t.Errorf("Expected single to be nil, got %v", single)
		}
	})

	t.Run("positional args are used as fallback", func(t *testing.T) {
		positional := []string{"pos1", "pos2"}
		series, single := v.handleCmds(nil, positional)

		if series != nil {
			t.Errorf("Expected series to be nil, got %v", series)
		}
		if !reflect.DeepEqual(single, positional) {
			t.Errorf("Expected single to be %v, got %v", positional, single)
		}
	})
}

func TestParseRegex(t *testing.T) {
	t.Run("parses comma-separated patterns", func(t *testing.T) {
		patterns := parseRegex("p1, p2 ,p3")
		expected := []string{"p1", "p2", "p3"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected %v, got %v", expected, patterns)
		}
	})

	t.Run("returns default patterns when empty", func(t *testing.T) {
		patterns := parseRegex("")
		expected := []string{".*\\.go$", "^go\\.mod$", "^go\\.sum$"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected default patterns, got %v", patterns)
		}
	})
}

func TestParseEnv(t *testing.T) {
	t.Run("parses comma-separated env vars", func(t *testing.T) {
		envMap := parseEnv("K1=V1, K2=V2 , K3=V3")
		expected := map[string]string{"K1": "V1", "K2": "V2", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})

	t.Run("handles invalid pairs gracefully", func(t *testing.T) {
		envMap := parseEnv("K1=V1,invalid,K3=V3")
		expected := map[string]string{"K1": "V1", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})
}
