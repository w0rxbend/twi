package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDesktopNotificationCommandByPlatform(t *testing.T) {
	name, args, ok := desktopNotificationCommand("linux", "Raid in #example", "15 raiders")
	if !ok || name != "notify-send" {
		t.Fatalf("linux command = %q %#v ok=%v, want notify-send", name, args, ok)
	}
	if got, want := args[len(args)-2], "Raid in #example"; got != want {
		t.Fatalf("linux title arg = %q, want %q", got, want)
	}
	if got, want := args[len(args)-1], "15 raiders"; got != want {
		t.Fatalf("linux body arg = %q, want %q", got, want)
	}

	name, args, ok = desktopNotificationCommand("darwin", "Raid in #example", "15 raiders")
	if !ok || name != "osascript" {
		t.Fatalf("darwin command = %q %#v ok=%v, want osascript", name, args, ok)
	}
	if got, want := args[len(args)-2], "Raid in #example"; got != want {
		t.Fatalf("darwin title arg = %q, want %q", got, want)
	}
	if got, want := args[len(args)-1], "15 raiders"; got != want {
		t.Fatalf("darwin body arg = %q, want %q", got, want)
	}

	name, args, ok = desktopNotificationCommand("windows", "Raid in #example", "15 raiders")
	if !ok || name != "powershell.exe" {
		t.Fatalf("windows command = %q %#v ok=%v, want powershell.exe", name, args, ok)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-EncodedCommand") {
		t.Fatalf("windows args missing encoded command: %#v", args)
	}
	if strings.Contains(joined, "Raid in #example") || strings.Contains(joined, "15 raiders") {
		t.Fatalf("windows args include raw notification text, want encoded command: %#v", args)
	}

	if _, _, ok := desktopNotificationCommand("plan9", "Raid", "body"); ok {
		t.Fatal("unsupported platform returned ok=true")
	}
}

func TestDefaultSystemNotifierUsesDesktopNotificationWhenAvailable(t *testing.T) {
	var bell bytes.Buffer
	var called bool
	notifier := defaultSystemNotifier{
		desktop: desktopNotifier{
			goos: "linux",
			lookPath: func(name string) (string, error) {
				if name != "notify-send" {
					t.Fatalf("lookPath name = %q, want notify-send", name)
				}
				return "/usr/bin/notify-send", nil
			},
			runCommand: func(ctx context.Context, path string, args ...string) error {
				called = true
				if path != "/usr/bin/notify-send" {
					t.Fatalf("runCommand path = %q, want /usr/bin/notify-send", path)
				}
				if len(args) == 0 || args[len(args)-1] != "15 raiders" {
					t.Fatalf("runCommand args = %#v, want notification body", args)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("runCommand context has no deadline")
				}
				return nil
			},
		},
		bell: terminalBellNotifier{w: &bell},
	}

	if err := notifier.Notify(context.Background(), SystemNotification{Title: "Raid in #example", Body: "15 raiders"}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if !called {
		t.Fatal("desktop command was not called")
	}
	if got := bell.String(); got != "" {
		t.Fatalf("bell output = %q, want empty when desktop notification succeeds", got)
	}
}

func TestDefaultSystemNotifierFallsBackToTerminalBell(t *testing.T) {
	var bell bytes.Buffer
	notifier := defaultSystemNotifier{
		desktop: desktopNotifier{
			goos: "linux",
			lookPath: func(string) (string, error) {
				return "", errors.New("notify-send missing")
			},
		},
		bell: terminalBellNotifier{w: &bell},
	}

	if err := notifier.Notify(context.Background(), SystemNotification{Title: "Raid in #example", Body: "15 raiders"}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if got, want := bell.String(), terminalBell; got != want {
		t.Fatalf("bell output = %q, want %q", got, want)
	}
}

func TestDesktopNotifierSanitizesNotificationText(t *testing.T) {
	var gotArgs []string
	notifier := desktopNotifier{
		goos:    "linux",
		timeout: time.Second,
		lookPath: func(string) (string, error) {
			return "/usr/bin/notify-send", nil
		},
		runCommand: func(_ context.Context, _ string, args ...string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	}

	err := notifier.Notify(context.Background(), SystemNotification{
		Title: "Raid\rin\n#example",
		Body:  "token=oauth:secret-token\n15 raiders",
	})
	if err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "\n") || strings.Contains(joined, "\r") {
		t.Fatalf("notification args contain control characters: %#v", gotArgs)
	}
	if strings.Contains(joined, "secret-token") || !strings.Contains(joined, "<redacted>") {
		t.Fatalf("notification args not redacted: %#v", gotArgs)
	}
}
