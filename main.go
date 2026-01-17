package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var version = "1.2.1"
var logger *Logger

// Args struct for command line arguments
type Args struct {
	CmdFlags       []string
	PositionalArgs []string
	Path           string
	Regex          string
	Env            string
	ConfigFile     string
	SaveFile       string
	Help           bool
	Debug          bool
	Version        bool
	Save           bool
}

// main is the entry point of the application
func main() {
	args := os.Args[1:]

	cli := parseArgs(args)

	// Set severity level based on debug flag
	bootstrapSeverity := SeverityWarn
	if cli.Debug {
		bootstrapSeverity = SeverityDebug
	}
	logger = newLogger(bootstrapSeverity)

	// Print startup message
	fmt.Print(purple("\n--------------\n"))
	fmt.Printf("%sVai v%s%s\n", ColorPurple, version, ColorPurple)
	fmt.Print(purple("--------------\n\n"))

	// Print current version and exit
	if cli.Version {
		os.Exit(0)
	}

	// Init vai
	v, err := newVai(cli)
	if err != nil {
		logger.log(SeverityError, OpError, "Failed to initialize Vai: %v", err)
		printHelp()
		os.Exit(1)
	}

	// Initialize logger based on vai config and context
	logger = newLogger(parseSeverity(v.Config.Severity))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Debug print current vai config
	if v.Config.Severity == SeverityDebug.String() {
		printConfig(v)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.log(SeverityDebug, OpSuccess, "Shutdown signal received")
		cancel()
	}()

	// Start the watcher in a goroutine
	var wg sync.WaitGroup
	wg.Go(func() {
		v.startWatch(ctx)
	})
	logger.log(SeverityWarn, OpSuccess, "File watcher started...")

	// Wait for the context to be canceled
	<-ctx.Done()

	// Wait for the watcher to finish
	wg.Wait()
	logger.log(SeverityInfo, OpWarn, "Shutting down...")
	v.manager.stop()

	// Save configuration if requested
	if cli.Save {
		logger.log(SeverityInfo, OpWarn, "Saving configuration to %s...", cli.SaveFile)
		if err := v.save(cli.SaveFile); err != nil {
			logger.log(SeverityError, OpError, "Failed to save config file: %v", err)
		}
		logger.log(SeverityInfo, OpSuccess, "Configuration saved successfully")
	}
}

// handleCmd parses the command flag
func (c *Args) handleCmd(attachedValue string, args []string, currentIndex int, knownFlagsWithArg, knownBoolFlags map[string]bool, shortFlags map[string]string) int {
	if attachedValue != "" {
		c.CmdFlags = append(c.CmdFlags, attachedValue)
		return currentIndex
	}
	var cmd string
	var newIndex int
	cmd, newIndex = parseCmdFlag(args, currentIndex, knownFlagsWithArg, knownBoolFlags, shortFlags)
	if cmd != "" {
		c.CmdFlags = append(c.CmdFlags, cmd)
	}
	return newIndex
}

// handleArgFlag parses flags with arguments
func (c *Args) handleArgFlag(flagName, attachedValue string, args []string, currentIndex int) int {
	var value string
	newIndex := currentIndex
	if attachedValue != "" {
		value = attachedValue
	} else {
		value, newIndex = parseValueFlag(args, currentIndex)
	}
	switch flagName {
	case "regex":
		c.Regex = value
	case "env":
		c.Env = value
	case "path":
		c.Path = value
	}
	return newIndex
}

// handleBoolFlag parses boolean flags
func (c *Args) handleBoolFlag(flagName string) {
	switch flagName {
	case "help":
		c.Help = true
	case "debug":
		c.Debug = true
	case "version":
		c.Version = true
	case "save":
		c.Save = true
	}
}

// parseArgs parses command line arguments
func parseArgs(args []string) *Args {
	c := &Args{
		ConfigFile: "vai.yml",
		SaveFile:   "vai.yml",
	}

	knownFlagsWithArg := map[string]bool{
		"cmd": true, "path": true, "env": true, "regex": true,
	}
	knownBoolFlags := map[string]bool{
		"help": true, "debug": true, "version": true, "save": true,
	}
	shortFlags := map[string]string{
		"c": "cmd", "p": "path", "e": "env", "r": "regex", "s": "save",
		"h": "help", "d": "debug", "v": "version",
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		isKnownFlag, flagName, attachedValue := identifyFlag(arg, knownFlagsWithArg, knownBoolFlags, shortFlags)

		if isKnownFlag {
			if flagName == "cmd" {
				i = c.handleCmd(attachedValue, args, i, knownFlagsWithArg, knownBoolFlags, shortFlags)
			} else if knownFlagsWithArg[flagName] {
				i = c.handleArgFlag(flagName, attachedValue, args, i)
			} else if knownBoolFlags[flagName] {
				c.handleBoolFlag(flagName)
			}
		} else {
			// The rest of the args belong to the cmd
			c.PositionalArgs = args[i:]
			break
		}
		i++
	}
	return c
}

// identifyFlag checks if an argument is a known flag
func identifyFlag(arg string, knownFlagsWithArg, knownBoolFlags map[string]bool, shortFlags map[string]string) (bool, string, string) {
	name := ""
	value := ""
	var found bool
	if name, found = strings.CutPrefix(arg, "--"); found {
	} else if name, found = strings.CutPrefix(arg, "-"); found {
	} else {
		return false, "", ""
	}

	// Check for =
	if before, after, found := strings.Cut(name, "="); found {
		name = before
		value = after
	}

	// Check long flags
	if knownFlagsWithArg[name] || knownBoolFlags[name] {
		return true, name, value
	}

	// Check short flags
	if longName, ok := shortFlags[name]; ok {
		return true, longName, value
	}

	return false, "", ""
}

// parseCmdFlag extracts the command from arguments
func parseCmdFlag(args []string, currentIndex int, knownFlagsWithArg, knownBoolFlags map[string]bool, shortFlags map[string]string) (string, int) {
	var cmdParts []string
	i := currentIndex + 1
	for i < len(args) {
		nextArg := args[i]
		isNextArgAFlag, _, _ := identifyFlag(nextArg, knownFlagsWithArg, knownBoolFlags, shortFlags)

		if isNextArgAFlag {
			i--
			break
		}
		cmdParts = append(cmdParts, nextArg)
		i++
	}
	if len(cmdParts) > 0 {
		return strings.Join(cmdParts, " "), i
	}
	return "", i
}

// parseValueFlag extracts the value for a flag
func parseValueFlag(args []string, currentIndex int) (string, int) {
	if currentIndex+1 < len(args) && !strings.HasPrefix(args[currentIndex+1], "-") {
		return args[currentIndex+1], currentIndex + 1
	}
	return "", currentIndex
}

// printHelp prints usage help info
func printHelp() {
	// Usage
	fmt.Println(
		yellow("Usage:"),
		"vai",
		cyan("[flags]"),
		cyan("[command...]..."),
	)

	fmt.Println()
	fmt.Println("A tool to run commands when files change, configured via CLI or a vai.yml file")

	// Configuration Modes
	fmt.Println()
	fmt.Println(yellow("Configuration Modes:"))

	fmt.Println(
		"  1.",
		white("CLI Mode:"),
		"Provide a command directly (e.g., `vai go run .`)",
	)

	fmt.Println(
		"  2.",
		white("File Mode:"),
		"Use a vai.yml file for complex workflows (e.g., `vai`)",
	)

	// Flags
	fmt.Println()
	fmt.Println(yellow("Flags:"))

	fmt.Println(
		"  ",
		cyan("-c, --cmd"),
		"<command>",
		"Command to run. Can be specified multiple times",
	)

	fmt.Println(
		"  ",
		cyan("-p, --path"),
		"<path>",
		"Path to watch. (default: .)",
	)

	fmt.Println(
		"  ",
		cyan("-e, --env"),
		"<vars>",
		"KEY=VALUE environment variables",
	)

	fmt.Println(
		"  ",
		cyan("-r, --regex"),
		"<patterns>",
		"Glob patterns to watch",
	)

	fmt.Println(
		"  ",
		cyan("-s, --save"),
		"Save CLI flags to a new vai.yml file and exit",
	)

	fmt.Println(
		"  ",
		cyan("-h, --help"),
		"Show this help message",
	)
}

// printConfig prints the current config
func printConfig(v *Vai) {
	fmt.Println(yellow("--- Global Config ---"))
	fmt.Println(cyan("- Cooldown:"), v.Config.Cooldown)
	fmt.Println(cyan("- Batching Duration:"), v.Config.BatchingDuration)
	fmt.Println(cyan("- Buffer Size:"), v.Config.BufferSize)
	fmt.Println(cyan("- Severity:"), v.Config.Severity)
	fmt.Println(cyan("- Clear CLI:"), v.Config.ClearCli)

	fmt.Println(yellow("---------------------"))

	if len(v.Jobs) == 0 {
		return
	}

	fmt.Println()
	fmt.Println(yellow("--- Jobs ---"))

	for name, job := range v.Jobs {
		fmt.Println(cyan("- Job:"), name)

		if job.Trigger != nil {
			if len(job.Trigger.Paths) > 0 {
				fmt.Println(
					"  ",
					cyan("- Watch Paths:"),
					strings.Join(job.Trigger.Paths, ", "),
				)
			}

			if len(job.Trigger.Regex) > 0 {
				fmt.Println(
					"  ",
					cyan("- Inclusion Regex:"),
					strings.Join(job.Trigger.Regex, ", "),
				)
			}
		}

		if len(job.Series) > 0 {
			fmt.Println("  ", cyan("- Commands:"))

			var printSubJobs func([]Job, string)
			printSubJobs = func(jobs []Job, indent string) {
				for _, j := range jobs {
					if j.Cmd != "" {
						cmd := j.Cmd
						if len(j.Params) > 0 {
							cmd += " " + strings.Join(j.Params, " ")
						}
						fmt.Println(indent, white("- ", cmd))
					} else if len(j.Parallel) > 0 {
						fmt.Println(indent, white("- Parallel:"))
						printSubJobs(j.Parallel, indent+"  ")
					} else if len(j.Series) > 0 {
						fmt.Println(indent, white("- Series:"))
						printSubJobs(j.Series, indent+"  ")
					}
				}
			}
			printSubJobs(job.Series, "    ")
		}

		if len(job.Env) > 0 {
			fmt.Println("  ", cyan("- Environment:"))

			for key, val := range job.Env {
				fmt.Println(
					"    ",
					white("- ", key+":"),
					val,
				)
			}
		}
	}

	fmt.Println(yellow("------------"))
}
