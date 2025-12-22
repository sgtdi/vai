package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"testing"
)

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

func TestHandleConfig(t *testing.T) {
	t.Run("CLI arguments take precedence", func(t *testing.T) {
		cmdFlags := []string{"go run ."}
		vai := handleConfig(cmdFlags, nil, "./app", true, "", "", "vai.yml", false, false, false)

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

		vai := handleConfig(nil, nil, "", false, "", "", tmpfile.Name(), false, false, false)

		if vai.Config.Path != "/file" {
			t.Errorf("Expected path to be '/file', got '%s'", vai.Config.Path)
		}
		if _, ok := vai.Jobs["from-file"]; !ok {
			t.Error("Expected job 'from-file' to be loaded from the config file")
		}
	})

	t.Run("Debug flag is set correctly", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "vai.*.yaml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		vai := handleConfig(nil, nil, "", false, "", "", tmpfile.Name(), false, true, false)
		if !vai.Config.Debug {
			t.Error("Expected Debug to be true, but it was false")
		}
	})
}

func TestHandleCommands(t *testing.T) {
	t.Run("cmd flags take precedence", func(t *testing.T) {
		cmdFlags := []string{"cmd1", "cmd2"}
		positional := []string{"pos1", "pos2"}
		series, single := handleCommands(cmdFlags, positional)

		if !reflect.DeepEqual(series, cmdFlags) {
			t.Errorf("Expected series to be %v, got %v", cmdFlags, series)
		}
		if single != nil {
			t.Errorf("Expected single to be nil, got %v", single)
		}
	})

	t.Run("positional args are used as fallback", func(t *testing.T) {
		positional := []string{"pos1", "pos2"}
		series, single := handleCommands(nil, positional)

		if series != nil {
			t.Errorf("Expected series to be nil, got %v", series)
		}
		if !reflect.DeepEqual(single, positional) {
			t.Errorf("Expected single to be %v, got %v", positional, single)
		}
	})

	t.Run("positional args with flags are parsed correctly", func(t *testing.T) {
		positional := []string{"some-command", "--flag", "value"}
		series, single := handleCommands(nil, positional)

		if series != nil {
			t.Errorf("Expected series to be nil, got %v", series)
		}
		if !reflect.DeepEqual(single, positional) {
			t.Errorf("Expected single to be %v, got %v", positional, single)
		}
	})
}

func TestHandleRegex(t *testing.T) {
	t.Run("parses comma-separated patterns", func(t *testing.T) {
		patterns := handleRegex("p1, p2 ,p3")
		expected := []string{"p1", "p2", "p3"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected %v, got %v", expected, patterns)
		}
	})

	t.Run("returns default patterns when empty", func(t *testing.T) {
		patterns := handleRegex("")
		expected := []string{".*\\.go$", "^go\\.mod$", "^go\\.sum$"}
		if !reflect.DeepEqual(patterns, expected) {
			t.Errorf("Expected default patterns, got %v", patterns)
		}
	})
}

func TestHandleEnv(t *testing.T) {
	t.Run("parses comma-separated env vars", func(t *testing.T) {
		envMap := handleEnv("K1=V1, K2=V2 , K3=V3")
		expected := map[string]string{"K1": "V1", "K2": "V2", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})

	t.Run("handles invalid pairs gracefully", func(t *testing.T) {
		envMap := handleEnv("K1=V1,invalid,K3=V3")
		expected := map[string]string{"K1": "V1", "K3": "V3"}
		if !reflect.DeepEqual(envMap, expected) {
			t.Errorf("Expected %v, got %v", expected, envMap)
		}
	})
}
