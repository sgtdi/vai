package main

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Job is the unified struct for any unit of work
type Job struct {
	Name     string            `yaml:"-"`
	Cmd      string            `yaml:"cmd,omitempty"`
	Params   []string          `yaml:"params,omitempty"`
	Series   []Job             `yaml:"series,omitempty"`
	Parallel []Job             `yaml:"parallel,omitempty"`
	Before   []Job             `yaml:"before,omitempty"`
	After    []Job             `yaml:"after,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Trigger  *Trigger          `yaml:"on,omitempty"`
}

// Trigger defines file paths and regex patterns to watch on
type Trigger struct {
	Paths []string `yaml:"paths,omitempty"`
	Regex []string `yaml:"regex,omitempty"`
}

// FromFile loads a Workflow from a YAML configuration file
func FromFile(filePath string, path string) (*Vai, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var vai Vai
	if err := yaml.Unmarshal(data, &vai); err != nil {
		return nil, err
	}
	for name, job := range vai.Jobs {
		job.Name = name
		vai.Jobs[name] = job
	}
	if path != "" {
		vai.Config.Path = path
	}
	return &vai, nil
}

// FromCLI creates a Workflow object from the parsed command-line flags
func FromCLI(seriesCmds []string, singleCmd []string, path string, patterns []string, env map[string]string) *Vai {
	var taskActions []Job

	if len(singleCmd) > 0 {
		// Positional arguments are treated as a single command
		cmd := singleCmd[0]
		params := singleCmd[1:]
		taskActions = append(taskActions, Job{Cmd: cmd, Params: params})
	} else {
		// --cmd flags are treated as a series of commands
		for _, cmdStr := range seriesCmds {
			parts := strings.Fields(cmdStr)
			cmd := ""
			params := []string{}
			if len(parts) > 0 {
				cmd = parts[0]
				params = parts[1:]
			}
			taskActions = append(taskActions, Job{Cmd: cmd, Params: params})
		}
	}

	jobAction := Job{
		Series: taskActions,
		Env:    env,
		Trigger: &Trigger{
			Paths: []string{path},
			Regex: patterns,
		},
	}

	return &Vai{
		Config: Config{
			Path: path,
		},
		Jobs: map[string]Job{
			"default": jobAction,
		},
	}
}

// UnmarshalYAML is the custom parser for the Action struct
func (a *Job) UnmarshalYAML(node *yaml.Node) error {
	// Try to unmarshal as a command string
	var simpleCmd string
	if err := node.Decode(&simpleCmd); err == nil {
		parts := strings.Fields(simpleCmd)
		if len(parts) > 0 {
			a.Cmd = parts[0]
			a.Params = parts[1:]
		}
		return nil
	}

	// Unmarshal it into a temporary struct to avoid recursion
	var raw struct {
		Name     string            `yaml:"name,omitempty"`
		Cmd      string            `yaml:"cmd,omitempty"`
		Params   []string          `yaml:"params,omitempty"`
		Series   []Job             `yaml:"series,omitempty"`
		Parallel []Job             `yaml:"parallel,omitempty"`
		Before   []Job             `yaml:"before,omitempty"`
		After    []Job             `yaml:"after,omitempty"`
		Env      map[string]string `yaml:"env,omitempty"`
		Trigger  *Trigger          `yaml:"trigger,omitempty"` // Deprecated: use On instead
		On       *Trigger          `yaml:"on,omitempty"`
	}

	if err := node.Decode(&raw); err != nil {
		return err
	}

	// Validate that only one of cmd, series, or parallel is set
	types := 0
	if raw.Cmd != "" {
		types++
	}
	if len(raw.Series) > 0 {
		types++
	}
	if len(raw.Parallel) > 0 {
		types++
	}
	if types > 1 {
		return &yaml.TypeError{Errors: []string{"action map can only contain one of 'cmd', 'series', or 'parallel' keys"}}
	}

	// Assign the fields from the temporary struct to the actual Job struct
	a.Name = raw.Name
	a.Cmd = raw.Cmd
	a.Params = raw.Params
	a.Series = raw.Series
	a.Parallel = raw.Parallel
	a.Before = raw.Before
	a.After = raw.After
	a.Env = raw.Env
	if raw.On != nil {
		a.Trigger = raw.On
	} else if raw.Trigger != nil {
		a.Trigger = raw.Trigger
	}

	return nil
}
