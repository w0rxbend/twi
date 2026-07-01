package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/w0rxbend/twi/internal/app"
	"github.com/w0rxbend/twi/internal/config"
)

const usage = `twi is a terminal Twitch chat client.

Usage:
  twi chat [--channel name] [--mock]
  twi config show
  twi config path
  twi doctor
  twi login

Environment:
  TWI_TWITCH_USERNAME
  TWI_TWITCH_OAUTH_TOKEN
  TWI_TWITCH_CLIENT_ID
  TWI_TWITCH_CLIENT_SECRET
  TWI_DEFAULT_CHANNELS
  TWI_ENABLE_KITTY_IMAGES
  TWI_IMAGE_MODE
  TWI_AVATAR_MODE
  TWI_EMOJI_MODE
  TWI_EMOTE_MODE
  TWI_ANIMATION_MODE
`

// Run executes the command line entrypoint. It returns a process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "chat":
		return runChat(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "login":
		fmt.Fprintln(stderr, "twi login is planned but not implemented in this bootstrap slice")
		return 2
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func runChat(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var channels channelFlags
	var cfgPath string
	var mock bool
	fs.Var(&channels, "channel", "Twitch channel to join; repeat for multiple channels")
	fs.StringVar(&cfgPath, "config", "", "config file path")
	fs.BoolVar(&mock, "mock", false, "run against the built-in mock chat source")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	overrides := config.Overrides{
		ConfigPath: cfgPath,
		Channels:   []string(channels),
	}
	cfg, err := config.Load(os.Environ(), overrides)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}
	if len(channels) > 0 {
		cfg.DefaultChannels = []string(channels)
	}

	if mock {
		if err := app.RunMock(stdout, cfg); err != nil {
			fmt.Fprintf(stderr, "mock chat: %v\n", err)
			return 1
		}
		return 0
	}

	if len(cfg.DefaultChannels) == 0 {
		fmt.Fprintln(stderr, "no channel configured; pass --channel or set TWI_DEFAULT_CHANNELS")
		return 2
	}
	if cfg.Twitch.Username == "" || cfg.Twitch.OAuthToken == "" {
		fmt.Fprintln(stderr, "missing Twitch credentials; set TWI_TWITCH_USERNAME and TWI_TWITCH_OAUTH_TOKEN or run `twi chat --mock`")
		return 2
	}

	fmt.Fprintln(stderr, "real Twitch chat transport is planned but not implemented in this bootstrap slice")
	return 2
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: twi config show|path")
		return 2
	}

	switch args[0] {
	case "path":
		path, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintf(stderr, "config path: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, path)
		return 0
	case "show":
		fs := flag.NewFlagSet("config show", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var cfgPath string
		fs.StringVar(&cfgPath, "config", "", "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		cfg, err := config.Load(os.Environ(), config.Overrides{ConfigPath: cfgPath})
		if err != nil {
			fmt.Fprintf(stderr, "load config: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, cfg.RedactedString())
		return 0
	default:
		fmt.Fprintf(stderr, "unknown config command %q\n", args[0])
		return 2
	}
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfgPath string
	fs.StringVar(&cfgPath, "config", "", "config file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(os.Environ(), config.Overrides{ConfigPath: cfgPath})
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	report := app.Doctor(cfg)
	for _, check := range report.Checks {
		status := "ok"
		if !check.OK {
			status = "warn"
		}
		fmt.Fprintf(stdout, "[%s] %s: %s\n", status, check.Name, check.Detail)
	}
	return 0
}

type channelFlags []string

func (f *channelFlags) String() string {
	return strings.Join(*f, ",")
}

func (f *channelFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("channel cannot be empty")
	}
	*f = append(*f, strings.TrimPrefix(value, "#"))
	return nil
}
