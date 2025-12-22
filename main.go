package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

// Config options for file vai.yaml
type Config struct {
	Path             string        `yaml:"path"`
	BufferSize       int           `yaml:"bufferSize"`
	LogLevel         string        `yaml:"logLevel"`
	Cooldown         time.Duration `yaml:"cooldown"`
	BatchingDuration time.Duration `yaml:"batchingDuration"`
	Debug            bool          `yaml:"debug,omitempty"`
}

type flags []string

func (i *flags) String() string { return strings.Join(*i, ", ") }

func (i *flags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var version = "1.1.0"

func main() {
	args := os.Args[1:]

	var cmdFlags []string
	var positionalArgs []string
	var path, regex, env, file, save string = ".", "", "", "watch.yml", "watch.yml"
	var help, debug, quiet, versionFlag bool
	var pathIsSet, saveIsSet bool

	knownFlagsWithArg := map[string]bool{
		"cmd": true, "path": true, "env": true, "regex": true, "save": true, "file": true,
	}
	knownBoolFlags := map[string]bool{
		"help": true, "debug": true, "quiet": true, "version": true,
	}
	shortFlags := map[string]string{
		"c": "cmd", "p": "path", "e": "env", "r": "regex", "s": "save", "f": "file",
		"h": "help", "d": "debug", "q": "quiet", "v": "version",
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		isKnownFlag := false
		var flagName string
		if name, found := strings.CutPrefix(arg, "--"); found {
			if knownFlagsWithArg[name] || knownBoolFlags[name] {
				isKnownFlag = true
				flagName = name
			}
		} else if name, found := strings.CutPrefix(arg, "-"); found {
			if longName, ok := shortFlags[name]; ok {
				isKnownFlag = true
				flagName = longName
			}
		}

		if isKnownFlag {
			if flagName == "cmd" {
				var cmdParts []string
				i++
				for i < len(args) {
					nextArg := args[i]
					isNextArgAFlag := false
					if strings.HasPrefix(nextArg, "-") {
						nextFlagName := strings.TrimLeft(nextArg, "-")
						if knownFlagsWithArg[nextFlagName] || knownBoolFlags[nextFlagName] {
							isNextArgAFlag = true
						}
					}

					if isNextArgAFlag {
						i--
						break
					}
					cmdParts = append(cmdParts, nextArg)
					i++
				}
				if len(cmdParts) > 0 {
					cmdFlags = append(cmdFlags, strings.Join(cmdParts, " "))
				}
			} else if knownFlagsWithArg[flagName] {
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					value := args[i+1]
					switch flagName {
					case "path":
						path = value
						pathIsSet = true
					case "regex":
						regex = value
					case "env":
						env = value
					case "file":
						file = value
					case "save":
						save = value
						saveIsSet = true
					}
					i++
				}
			} else if knownBoolFlags[flagName] {
				switch flagName {
				case "help":
					help = true
				case "debug":
					debug = true
				case "quiet":
					quiet = true
				case "version":
					versionFlag = true
				}
			}
		} else {
			positionalArgs = append(positionalArgs, arg)
		}
		i++
	}

	fmt.Printf("\n%s--------------%s\n", colorPurple, colorReset)
	fmt.Printf("%sVai - v%s%s\n", colorPurple, version, colorReset)
	fmt.Printf("%s--------------%s\n\n", colorPurple, colorReset)

	if versionFlag {
		os.Exit(0)
	}

	watch := handleConfig(
		cmdFlags,
		positionalArgs,
		path,
		pathIsSet,
		regex,
		env,
		file,
		help,
		debug,
		quiet,
	)

	Log(SeverityInfo, "Starting file watch...")
	watch.jobManager = NewJobManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		Log(SeverityInfo, "Shutdown signal received.")
		cancel()
	}()

	// Start the watcher in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		startWatch(ctx, watch)
	}()

	// Wait for the context to be canceled
	<-ctx.Done()

	// Wait for the watcher to finish
	wg.Wait()

	Log(SeverityInfo, "Shutting down. Stopping all running jobs...")
	watch.jobManager.StopAll(watch.Config.Debug)

	if saveIsSet {
		Logf(SeverityInfo, "Saving configuration to %s on exit...", save)
		if err := watch.Save(save); err != nil {
			Logf(SeverityError, "Failed to save config file on exit: %v", err)
		} else {
			Log(SeveritySuccess, "Configuration saved successfully.")
		}
	}

	Log(SeverityInfo, "Vai shutting down")
}

// handleConfig parse config struct with all possible flags and args
func handleConfig(cmdFlags []string, positionalArgs []string, path string, pathIsSet bool, regex, env, filePath string, help, debug, quiet bool) *Vai {
	// Set quiet flag
	isQuiet = quiet
	// Handle help flag
	handleHelp(help)

	var watch *Vai
	var err error

	// Prioritize CLI commands over config
	if len(cmdFlags) > 0 || len(positionalArgs) > 0 {
		seriesCmds, singleCmd := handleCommands(cmdFlags, positionalArgs)
		patterns := handleRegex(regex)
		envMap := handleEnv(env)
		watch = FromCLI(seriesCmds, singleCmd, path, patterns, envMap)
	} else {
		// Fallback to config with no cmds
		isDefaultConfig := filePath == "watch.yml"
		if !isDefaultConfig || fileExists(filePath) {
			watch, err = FromFile(filePath, path, pathIsSet)
			if err != nil {
				Logf(SeverityError, "Failed to load config file: %v", err)
				os.Exit(1)
			}
		} else {
			// If none show help
			Log(SeverityError, "No config file found and no command given.")
			handleHelp(true)
		}
	}

	watch.Config.Debug = debug
	watch.SetDefaults()

	if watch.Config.Debug && !isQuiet {
		printConfig(watch)
	}

	return watch
}

// printConfig prints the current config
func printConfig(w *Vai) {

	fmt.Printf("%s--- Global Config ---%s\n", colorYellow, colorReset)
	fmt.Printf("%s- Path:%s %s\n", colorCyan, colorReset, w.Config.Path)
	fmt.Printf("%s- Cooldown:%s %s\n", colorCyan, colorReset, w.Config.Cooldown)
	fmt.Printf("%s- Batching Duration:%s %s\n", colorCyan, colorReset, w.Config.BatchingDuration)
	fmt.Printf("%s- Buffer Size:%s %d\n", colorCyan, colorReset, w.Config.BufferSize)
	fmt.Printf("%s- Log Level:%s %s\n", colorCyan, colorReset, w.Config.LogLevel)
	fmt.Printf("%s---------------------%s\n", colorYellow, colorReset)

	if len(w.Jobs) > 0 {
		fmt.Printf("\n%s--- Jobs ---%s\n", colorYellow, colorReset)
		for name, job := range w.Jobs {
			fmt.Printf("%s- Job:%s %s\n", colorCyan, colorReset, name)
			if job.On != nil {
				if len(job.On.Paths) > 0 {
					fmt.Printf("  %s- Watch Paths:%s %s\n", colorCyan, colorReset, strings.Join(job.On.Paths, ", "))
				}
				if len(job.On.Regex) > 0 {
					fmt.Printf("  %s- Inclusion Regex:%s %s\n", colorCyan, colorReset, strings.Join(job.On.Regex, ", "))
				}
			}

			if len(job.Series) > 0 {
				fmt.Printf("  %s- Commands:%s\n", colorCyan, colorReset)
				for _, seriesJob := range job.Series {
					cmd := seriesJob.Cmd
					if len(seriesJob.Params) > 0 {
						cmd += " " + strings.Join(seriesJob.Params, " ")
					}
					fmt.Printf("    %s- %s%s\n", colorWhite, cmd, colorReset)
				}
			}

			if len(job.Env) > 0 {
				fmt.Printf("  %s- Environment:%s\n", colorCyan, colorReset)
				for key, val := range job.Env {
					fmt.Printf("    %s- %s:%s %s\n", colorWhite, key, colorReset, val)
				}
			}
		}
		fmt.Printf("%s------------%s\n", colorYellow, colorReset)
	}
}

// handleCommands determines the commands to run
func handleCommands(cmdFlags []string, positionalArgs []string) ([]string, []string) {
	if len(cmdFlags) > 0 {
		// --cmd flags take precedence
		return cmdFlags, nil
	}
	if len(positionalArgs) > 0 {
		// Positional args are treated as a single command with args
		return nil, positionalArgs
	}
	Log(SeverityError, "No command provided. Use --help for usage details.")
	os.Exit(1)
	return nil, nil
}

// handleRegex determines the file patterns to watch
func handleRegex(regexFlag string) []string {
	if regexFlag != "" {
		patterns := strings.Split(regexFlag, ",")
		for i, p := range patterns {
			patterns[i] = strings.TrimSpace(p)
		}
		return patterns
	}
	// Default patterns
	return []string{".*\\.go$", "^go\\.mod$", "^go\\.sum$"}
}

// handleEnv parses the env variables
func handleEnv(envFlag string) map[string]string {
	envMap := make(map[string]string)
	if envFlag != "" {
		pairs := strings.Split(envFlag, ",")
		for _, pair := range pairs {
			trimmedPair := strings.TrimSpace(pair)
			if key, value, found := strings.Cut(trimmedPair, "="); found {
				envMap[key] = value
			}
		}
	}
	return envMap
}

// handleHelp prints usage help info
func handleHelp(helpFlag bool) {
	if !helpFlag {
		return
	}

	fmt.Printf("%sUsage:%s vai %s[flags]%s %s[command...]...%s\n", colorYellow, colorReset, colorCyan, colorReset, colorCyan, colorReset)
	fmt.Printf("   or: watch %s--file%s <file>\n", colorCyan, colorReset)
	fmt.Println("\nA tool to run commands when files change, configured via CLI or a YAML file.")

	fmt.Printf("\n%sConfiguration Modes:%s\n", colorYellow, colorReset)
	fmt.Printf("  1. %sCLI Mode:%s Provide a command directly (e.g., `watch go run .`).\n", colorWhite, colorReset)
	fmt.Printf("  2. %sFile Mode:%s Use a YAML file for complex workflows (e.g., `watch --file watch.yml`).\n", colorWhite, colorReset)

	fmt.Printf("\n%sFlags:%s\n", colorYellow, colorReset)
	fmt.Printf("  %s-c, --cmd%s <command>      Command to run. Can be specified multiple times.\n", colorCyan, colorReset)
	fmt.Printf("  %s-p, --path%s <path>        Path to watch. (default: .)\n", colorCyan, colorReset)
	fmt.Printf("  %s-e, --env%s <vars>         KEY=VALUE environment variables.\n", colorCyan, colorReset)
	fmt.Printf("  %s-r, --regex%s <patterns>   Glob patterns to watch.\n", colorCyan, colorReset)
	fmt.Printf("  %s-s, --save%s <file>        Save CLI flags to a new YAML file and exit.\n", colorCyan, colorReset)
	fmt.Printf("  %s-f, --file%s <file>        Load configuration from a YAML file.\n", colorCyan, colorReset)
	fmt.Printf("  %s-q, --quiet%s              Disable all logging output.\n", colorCyan, colorReset)
	fmt.Printf("  %s-h, --help%s               Show this help message.\n", colorCyan, colorReset)
	os.Exit(0)
}

// fileExists checks if a file exists and is not a dir
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
