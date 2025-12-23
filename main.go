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

// Config options for file vai.yml
type Config struct {
	Path             string        `yaml:"path"`
	BufferSize       int           `yaml:"bufferSize"`
	LogLevel         string        `yaml:"logLevel"`
	Cooldown         time.Duration `yaml:"cooldown"`
	BatchingDuration time.Duration `yaml:"batchingDuration"`
	Debug            bool          `yaml:"debug,omitempty"`
	ClearConsole     *bool         `yaml:"clearConsole,omitempty"`
}

var version = "1.1.0"

func main() {
	args := os.Args[1:]

	var cmdFlags []string
	var positionalArgs []string
	var path, regex, env, configFile, saveFile string = ".", "", "", "vai.yml", "vai.yml"
	var help, debug, quiet, versionFlag, saveIsSet bool
	var pathIsSet bool

	knownFlagsWithArg := map[string]bool{
		"cmd": true, "path": true, "env": true, "regex": true,
	}
	knownBoolFlags := map[string]bool{
		"help": true, "debug": true, "quiet": true, "version": true, "save": true,
	}
	shortFlags := map[string]string{
		"c": "cmd", "p": "path", "e": "env", "r": "regex", "s": "save",
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
				case "save":
					saveIsSet = true
				}
			}
		} else {
			// This is the first, the rest of the args belong to the command.
			positionalArgs = args[i:]
			break
		}
		i++
	}

	fmt.Printf("\n%s--------------%s\n", ColorPurple, ColorReset)
	fmt.Printf("%sVai - v%s%s\n", ColorPurple, version, ColorReset)
	fmt.Printf("%s--------------%s\n\n", ColorPurple, ColorReset)

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
		configFile,
		help,
		debug,
		quiet,
	)

	if debug {
		Log(SeverityInfo, "Starting file watching...")
	}
	watch.jobManager = NewJobManager()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		if watch.Config.Debug {
			Log(SeverityInfo, "Shutdown signal received.")
		}
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

	Log(SeverityInfo, "Shutting down...")
	watch.jobManager.StopAll(watch.Config.Debug)

	if saveIsSet {
		if watch.Config.Debug {
			Logf(SeverityInfo, "Saving configuration to %s...", saveFile)
		}
		if err := watch.Save(saveFile); err != nil {
			Logf(SeverityError, "Failed to save config file: %v", err)
		} else if watch.Config.Debug {
			Log(SeveritySuccess, "Configuration saved successfully")
		}
	}
}

// handleConfig parse config struct with all possible flags and args
func handleConfig(cmdFlags []string, positionalArgs []string, path string, pathIsSet bool, regex, env, configFile string, help, debug, quiet bool) *Vai {
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
		if fileExists(configFile) {
			watch, err = FromFile(configFile, path, pathIsSet)
			if err != nil {
				Logf(SeverityError, "Failed to load config file: %v", err)
				os.Exit(1)
			}
			Logf(SeverityInfo, "Using config %s%s%s", ColorCyan, configFile, ColorReset)
		} else {
			// If none show help
			Log(SeverityError, "No config file found and no command given.")
			handleHelp(true)
		}
	}

	if watch == nil {
		Log(SeverityError, "Internal error: watch configuration not initialized.")
		os.Exit(1)
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

	fmt.Printf("%s--- Global Config ---%s\n", ColorYellow, ColorReset)
	fmt.Printf("%s- Path:%s %s\n", ColorCyan, ColorReset, w.Config.Path)
	fmt.Printf("%s- Cooldown:%s %s\n", ColorCyan, ColorReset, w.Config.Cooldown)
	fmt.Printf("%s- Batching Duration:%s %s\n", ColorCyan, ColorReset, w.Config.BatchingDuration)
	fmt.Printf("%s- Buffer Size:%s %d\n", ColorCyan, ColorReset, w.Config.BufferSize)
	fmt.Printf("%s- Log Level:%s %s\n", ColorCyan, ColorReset, w.Config.LogLevel)
	fmt.Printf("%s---------------------%s\n", ColorYellow, ColorReset)

	if len(w.Jobs) > 0 {
		fmt.Printf("\n%s--- Jobs ---%s\n", ColorYellow, ColorReset)
		for name, job := range w.Jobs {
			fmt.Printf("%s- Job:%s %s\n", ColorCyan, ColorReset, name)
			if job.Trigger != nil {
				if len(job.Trigger.Paths) > 0 {
					fmt.Printf("  %s- Watch Paths:%s %s\n", ColorCyan, ColorReset, strings.Join(job.Trigger.Paths, ", "))
				}
				if len(job.Trigger.Regex) > 0 {
					fmt.Printf("  %s- Inclusion Regex:%s %s\n", ColorCyan, ColorReset, strings.Join(job.Trigger.Regex, ", "))
				}
			}

			if len(job.Series) > 0 {
				fmt.Printf("  %s- Commands:%s\n", ColorCyan, ColorReset)
				for _, seriesJob := range job.Series {
					cmd := seriesJob.Cmd
					if len(seriesJob.Params) > 0 {
						cmd += " " + strings.Join(seriesJob.Params, " ")
					}
					fmt.Printf("    %s- %s%s\n", ColorWhite, cmd, ColorReset)
				}
			}

			if len(job.Env) > 0 {
				fmt.Printf("  %s- Environment:%s\n", ColorCyan, ColorReset)
				for key, val := range job.Env {
					fmt.Printf("    %s- %s:%s %s\n", ColorWhite, key, ColorReset, val)
				}
			}
		}
		fmt.Printf("%s------------%s\n", ColorYellow, ColorReset)
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

	fmt.Printf("%sUsage:%s vai %s[flags]%s %s[command...]...%s\n", ColorYellow, ColorReset, ColorCyan, ColorReset, ColorCyan, ColorReset)
	fmt.Println("\nA tool to run commands when files change, configured via CLI or a vai.yml file.")

	fmt.Printf("\n%sConfiguration Modes:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  1. %sCLI Mode:%s Provide a command directly (e.g., `vai go run .`).\n", ColorWhite, ColorReset)
	fmt.Printf("  2. %sFile Mode:%s Use a vai.yml file for complex workflows (e.g., `vai`).\n", ColorWhite, ColorReset)

	fmt.Printf("\n%sFlags:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %s-c, --cmd%s <command>      Command to run. Can be specified multiple times.\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-p, --path%s <path>        Path to watch. (default: .)\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-e, --env%s <vars>         KEY=VALUE environment variables.\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-r, --regex%s <patterns>   Glob patterns to watch.\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-s, --save%s               Save CLI flags to a new vai.yml file and exit.\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-q, --quiet%s              Disable all logging output.\n", ColorCyan, ColorReset)
	fmt.Printf("  %s-h, --help%s               Show this help message.\n", ColorCyan, ColorReset)
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
