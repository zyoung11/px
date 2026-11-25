package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"px/internal/daemon"
	"px/internal/logx"
	"px/internal/service"
	"px/internal/tui"
)

const version = "0.1.7"

const (
	colorMauve  = "\x1b[38;2;198;160;246m"
	colorBlue   = "\x1b[38;2;138;173;244m"
	colorGreen  = "\x1b[38;2;166;218;149m"
	colorYellow = "\x1b[38;2;238;212;159m"
	colorBold   = "\x1b[1m"
	colorReset  = "\x1b[0m"
)

// command is the shared data source for dispatch and help rendering.
type command struct {
	name    string
	summary string
	usage   string
	options [][2]string
	run     func(configPath string) int
}

var examples = map[string][]string{
	"run":       {"px run", "px run --config /etc/px/config.json"},
	"install":   {"sudo px install", "sudo px install --config /etc/px/config.json"},
	"uninstall": {"sudo px uninstall"},
}

var configOption = [2]string{"--config <path>", "state file path, default ~/.config/px/config.json"}

var commands = []command{
	{
		name:    "run",
		summary: "Run the daemon in the foreground",
		usage:   "px run [--config <path>]",
		options: [][2]string{configOption},
		run: func(configPath string) int {
			if !requireRoot() {
				return 1
			}
			if err := daemon.Run(configPath); err != nil {
				logx.Error("%v", err)
				return 1
			}
			return 0
		},
	},
	{
		name:    "install",
		summary: "Install the systemd service",
		usage:   "px install [--config <path>]",
		options: [][2]string{configOption},
		run:     runInstall,
	},
	{
		name:    "uninstall",
		summary: "Remove the systemd service",
		usage:   "px uninstall",
		run:     runUninstall,
	},
}

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

// dispatch resolves one command line and returns the process exit code.
func dispatch(args []string) int {
	if len(args) == 0 {
		return runTUI()
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "-v", "-V", "--version":
		fmt.Printf("%spx%s %s%s%s\n", colorBlue, colorReset, colorGreen, version, colorReset)
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		if hasHelpFlag(args[1:]) {
			printCommandUsage(c)
			return 0
		}
		configPath, err := parseConfigPath(args[1:])
		if err != nil {
			logx.Error("%v", err)
			printCommandUsage(c)
			return 1
		}
		return c.run(configPath)
	}
	logx.Error("unknown command %q", args[0])
	printUsage()
	return 1
}

// parseConfigPath reads the optional --config flag and rejects anything else.
func parseConfigPath(args []string) (string, error) {
	path := defaultConfigPath()
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--config needs a path")
			}
			i++
			path = args[i]
		case strings.HasPrefix(arg, "--config="):
			path = strings.TrimPrefix(arg, "--config=")
		default:
			return "", fmt.Errorf("unexpected argument %q", arg)
		}
	}
	return path, nil
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(dir, "px", "config.json")
}

// requireRoot reports an error and returns false unless the process runs
// as root, so privileged commands fail before any work starts.
func requireRoot() bool {
	if os.Geteuid() != 0 {
		logx.Error("root is required, run with sudo")
		return false
	}
	return true
}

func runTUI() int {
	tui.SetVersion(version)
	if err := tui.Run(); err != nil {
		logx.Error("%v", err)
		logx.Info("start the daemon first: systemctl start px")
		return 1
	}
	return 0
}

func runInstall(configPath string) int {
	if !requireRoot() {
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		logx.Error("%v", err)
		return 1
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		logx.Error("%v", err)
		return 1
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		logx.Error("%v", err)
		return 1
	}
	logx.Info("installing systemd service")
	if err := service.Install(exe, configPath); err != nil {
		logx.Error("%v", err)
		return 1
	}
	logx.Success("service installed, config at %s", configPath)
	return 0
}

func runUninstall(configPath string) int {
	if !requireRoot() {
		return 1
	}
	logx.Info("removing systemd service")
	if err := service.Uninstall(); err != nil {
		logx.Error("%v", err)
		return 1
	}
	logx.Success("service removed")
	return 0
}

// printUsage renders the overview page.
func printUsage() {
	fmt.Printf(
		"\n%s%spx%s — minimal mihomo controller\n\n"+
			"%sUSAGE:%s\n"+
			"  %spx%s\n"+
			"  %spx%s <command>\n\n"+
			"%sCOMMANDS:%s\n",
		colorBold, colorBlue, colorReset,
		colorBold+colorMauve, colorReset,
		colorBlue, colorReset,
		colorBlue, colorReset,
		colorBold+colorMauve, colorReset,
	)
	width := 0
	for _, c := range commands {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	for _, c := range commands {
		fmt.Printf("  %s%s%s %s\n", colorGreen, c.name+strings.Repeat(" ", width-len(c.name)), colorReset, c.summary)
	}
	flags := [][2]string{
		{"-h, --help", "Show this help page"},
		{"-v, -V, --version", "Show the version"},
	}
	fmt.Printf("\n%sOPTIONS:%s\n", colorBold+colorMauve, colorReset)
	flagWidth := 0
	for _, f := range flags {
		if len(f[0]) > flagWidth {
			flagWidth = len(f[0])
		}
	}
	for _, f := range flags {
		fmt.Printf("  %s%s%s %s\n", colorYellow,
			f[0]+strings.Repeat(" ", flagWidth-len(f[0])), colorReset, f[1])
	}
	fmt.Println()
}

// printCommandUsage renders one subcommand page.
func printCommandUsage(c command) {
	fmt.Printf(
		"\n%s%spx %s%s — %s%s\n\n"+
			"%sUSAGE:%s\n"+
			"  %s%s%s\n",
		colorBold, colorBlue, c.name, colorReset, c.summary, colorReset,
		colorBold+colorMauve, colorReset,
		colorBlue, c.usage, colorReset,
	)
	if len(c.options) > 0 {
		fmt.Printf("\n%sOPTIONS:%s\n", colorBold+colorMauve, colorReset)
		width := 0
		for _, o := range c.options {
			if len(o[0]) > width {
				width = len(o[0])
			}
		}
		for _, o := range c.options {
			fmt.Printf("  %s%s%s %s\n", colorYellow,
				o[0]+strings.Repeat(" ", width-len(o[0])), colorReset, o[1])
		}
	}
	if list := examples[c.name]; len(list) > 0 {
		fmt.Printf("\n%sEXAMPLES:%s\n", colorBold+colorMauve, colorReset)
		for _, e := range list {
			fmt.Printf("  %s%s%s\n", colorBlue, e, colorReset)
		}
	}
	fmt.Println()
}
