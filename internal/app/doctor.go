package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/twitch"
)

const (
	DoctorStatusOK   DoctorStatus = "ok"
	DoctorStatusWarn DoctorStatus = "warn"
)

var oauthPattern = regexp.MustCompile(`(?i)oauth:[^\s]+`)

type DoctorStatus string

type DoctorReport struct {
	Checks []DoctorCheck
}

type DoctorCheck struct {
	Name   string
	Status DoctorStatus
	Detail string
}

type DoctorOptions struct {
	Environ           []string
	CacheDir          string
	ConfigLoadError   error
	ReachabilityProbe ReachabilityProbe
	TokenValidator    twitch.TokenValidator
}

type ReachabilityProbe func(context.Context) error

func Doctor(ctx context.Context, cfg config.Config) DoctorReport {
	return DoctorWithOptions(ctx, cfg, DoctorOptions{
		Environ:           os.Environ(),
		ReachabilityProbe: ProbeTwitchIRCReachability,
	})
}

func DoctorWithOptions(ctx context.Context, cfg config.Config, opts DoctorOptions) DoctorReport {
	if opts.Environ == nil {
		opts.Environ = os.Environ()
	}
	if opts.ReachabilityProbe == nil {
		opts.ReachabilityProbe = ProbeTwitchIRCReachability
	}

	checks := []DoctorCheck{
		configPathCheck(cfg.Path, opts.ConfigLoadError),
		credentialCheck("twitch username", cfg.Twitch.Username, "live chat unavailable until TWI_TWITCH_USERNAME or TWITCH_USERNAME is set"),
		credentialCheck("oauth token", cfg.Twitch.OAuthToken, "live chat unavailable until TWI_TWITCH_OAUTH_TOKEN or TWITCH_ACCESS_TOKEN is set"),
		credentialCheck("refresh token", cfg.Twitch.RefreshToken, "auth refresh unavailable until TWI_TWITCH_REFRESH_TOKEN or TWITCH_REFRESH_TOKEN is set"),
		credentialCheck("client id", cfg.Twitch.ClientID, "optional Helix/API features unavailable"),
		credentialCheck("client secret", cfg.Twitch.ClientSecret, "optional OAuth client-secret flow unavailable"),
		channelsCheck(cfg.DefaultChannels),
		tokenValidationCheck(ctx, cfg, opts.TokenValidator),
		reachabilityCheck(ctx, opts.ReachabilityProbe),
		terminalCheck(opts.Environ),
		colorCheck(opts.Environ),
		mouseCheck(opts.Environ),
		kittyGraphicsCheck(cfg, opts.Environ),
		cacheCheck(opts.CacheDir),
		featureModesCheck(cfg.Features),
	}

	for i := range checks {
		checks[i].Detail = redactSensitive(checks[i].Detail, cfg)
	}
	return DoctorReport{Checks: checks}
}

func ProbeTwitchIRCReachability(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	dialer := net.Dialer{Timeout: 800 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", "irc.chat.twitch.tv:6697")
	if err != nil {
		return err
	}
	return conn.Close()
}

func configPathCheck(path string, loadErr error) DoctorCheck {
	if strings.TrimSpace(path) == "" {
		return warnCheck("config file", "path unavailable")
	}
	if loadErr != nil {
		return warnCheck("config file", fmt.Sprintf("%s (load failed: %v; using env/defaults)", path, loadErr))
	}
	info, err := os.Stat(path)
	switch {
	case err == nil && !info.IsDir():
		file, err := os.Open(path)
		if err != nil {
			return warnCheck("config file", fmt.Sprintf("%s (not readable: %v)", path, err))
		}
		if err := file.Close(); err != nil {
			return warnCheck("config file", fmt.Sprintf("%s (close failed: %v)", path, err))
		}
		return okCheck("config file", fmt.Sprintf("%s (readable)", path))
	case err == nil && info.IsDir():
		return warnCheck("config file", fmt.Sprintf("%s is a directory", path))
	case errors.Is(err, os.ErrNotExist):
		return warnCheck("config file", fmt.Sprintf("%s (not found; using env/defaults)", path))
	default:
		return warnCheck("config file", fmt.Sprintf("%s (%v)", path, err))
	}
}

func credentialCheck(name, value, missingDetail string) DoctorCheck {
	if strings.TrimSpace(value) == "" {
		return warnCheck(name, "missing; "+missingDetail)
	}
	return okCheck(name, "present")
}

func channelsCheck(channels []string) DoctorCheck {
	switch len(channels) {
	case 0:
		return warnCheck("channels", "none configured; pass --channel or set TWI_DEFAULT_CHANNELS")
	case 1:
		return okCheck("channels", "one configured")
	default:
		return warnCheck("channels", fmt.Sprintf("%d configured; live IRC currently supports one", len(channels)))
	}
}

func tokenValidationCheck(ctx context.Context, cfg config.Config, validator twitch.TokenValidator) DoctorCheck {
	if strings.TrimSpace(cfg.Twitch.OAuthToken) == "" {
		return warnCheck("token validation", "skipped; OAuth token missing")
	}
	if validator == nil {
		return warnCheck("token validation", "not available; required scopes "+tokenScopesCSV(twitch.RequiredIRCScopes())+" were not verified")
	}

	credentials := tokenCredentialsFromConfig(cfg.Twitch)
	validation, err := validator.ValidateToken(ctx, credentials)
	if err != nil {
		return warnCheck("token validation", fmt.Sprintf("failed: %v", err))
	}

	missing := validation.MissingScopes
	if len(missing) == 0 {
		missing = twitch.MissingRequiredIRCScopes(validation.Scopes)
	}

	if mismatch := tokenUsernameMismatch(cfg.Twitch.Username, validation.Identity.Login); mismatch != "" && validation.Status == twitch.TokenValidationValid {
		return warnCheck("token validation", mismatch)
	}

	switch validation.Status {
	case twitch.TokenValidationValid:
	case twitch.TokenValidationMalformed:
		return warnCheck("token validation", tokenValidationDetail(validation, "malformed OAuth token"))
	case twitch.TokenValidationExpired:
		return warnCheck("token validation", tokenValidationDetail(validation, "OAuth token expired; "+refreshAvailabilityDetail(validation.RefreshAvailable)))
	case twitch.TokenValidationWrongUser:
		return warnCheck("token validation", tokenValidationDetail(validation, usernameOwnershipDetail(cfg.Twitch.Username, validation.Identity.Login)))
	case twitch.TokenValidationMissingScope:
		if len(missing) > 0 {
			return warnCheck("token validation", "missing required scopes: "+tokenScopesCSV(missing))
		}
		return warnCheck("token validation", tokenValidationDetail(validation, "missing required IRC scope"))
	default:
		return warnCheck("token validation", tokenValidationDetail(validation, "token validation returned unknown state"))
	}

	if len(missing) > 0 {
		return warnCheck("token validation", "missing required scopes: "+tokenScopesCSV(missing))
	}
	return okCheck("token validation", "valid with required scopes "+tokenScopesCSV(twitch.RequiredIRCScopes()))
}

func reachabilityCheck(ctx context.Context, probe ReachabilityProbe) DoctorCheck {
	if probe == nil {
		return warnCheck("twitch reachability", "not checked")
	}
	if err := probe(ctx); err != nil {
		return warnCheck("twitch reachability", fmt.Sprintf("irc.chat.twitch.tv:6697 unreachable: %v", err))
	}
	return okCheck("twitch reachability", "irc.chat.twitch.tv:6697 reachable")
}

func terminalCheck(environ []string) DoctorCheck {
	term := envMap(environ)["TERM"]
	switch {
	case term == "":
		return warnCheck("terminal", "TERM missing; terminal capability detection is limited")
	case term == "dumb":
		return warnCheck("terminal", "TERM=dumb; rich TUI features may be unavailable")
	default:
		return okCheck("terminal", "TERM="+term)
	}
}

func colorCheck(environ []string) DoctorCheck {
	env := envMap(environ)
	term := env["TERM"]
	colorTerm := strings.ToLower(env["COLORTERM"])
	switch {
	case strings.Contains(colorTerm, "truecolor"), strings.Contains(colorTerm, "24bit"):
		return okCheck("terminal color", "true-color signal via COLORTERM")
	case strings.Contains(term, "truecolor"), strings.Contains(term, "24bit"), strings.Contains(term, "direct"):
		return okCheck("terminal color", "true-color signal via TERM")
	case strings.Contains(term, "256color"):
		return okCheck("terminal color", "256-color signal via TERM")
	default:
		return warnCheck("terminal color", "no true-color or 256-color signal; colors will use conservative fallbacks")
	}
}

func mouseCheck(environ []string) DoctorCheck {
	term := envMap(environ)["TERM"]
	if term == "" || term == "dumb" {
		return warnCheck("terminal mouse", "mouse support unknown; keyboard controls remain primary")
	}
	return okCheck("terminal mouse", "terminal advertises interactive capabilities; mouse behavior remains optional")
}

func kittyGraphicsCheck(cfg config.Config, environ []string) DoctorCheck {
	if !cfg.Features.EnableKittyImages || modeDisabled(cfg.Features.ImageMode) {
		return okCheck("kitty graphics", "disabled by config; text fallbacks active")
	}
	env := envMap(environ)
	switch {
	case env["KITTY_WINDOW_ID"] != "":
		return okCheck("kitty graphics", "Kitty signal detected via KITTY_WINDOW_ID")
	case strings.Contains(env["TERM"], "xterm-kitty"):
		return okCheck("kitty graphics", "Kitty signal detected via TERM")
	case strings.EqualFold(env["TERM_PROGRAM"], "ghostty"):
		return okCheck("kitty graphics", "Kitty-compatible signal detected via TERM_PROGRAM=ghostty")
	default:
		return warnCheck("kitty graphics", "no Kitty/Ghostty signal detected; image fallbacks active")
	}
}

func cacheCheck(cacheDir string) DoctorCheck {
	if strings.TrimSpace(cacheDir) == "" {
		defaultDir, err := config.DefaultCacheDir()
		if err != nil {
			return warnCheck("cache directory", fmt.Sprintf("path unavailable: %v", err))
		}
		cacheDir = defaultDir
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return warnCheck("cache directory", fmt.Sprintf("%s not writable: %v", cacheDir, err))
	}

	probePath := filepath.Join(cacheDir, ".twi-doctor-write-test")
	file, err := os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		probePath = filepath.Join(cacheDir, fmt.Sprintf(".twi-doctor-write-test-%d", os.Getpid()))
		file, err = os.OpenFile(probePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	}
	if err != nil {
		return warnCheck("cache directory", fmt.Sprintf("%s not writable: %v", cacheDir, err))
	}
	if _, err := file.Write([]byte("ok\n")); err != nil {
		_ = file.Close()
		_ = os.Remove(probePath)
		return warnCheck("cache directory", fmt.Sprintf("%s not writable: %v", cacheDir, err))
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(probePath)
		return warnCheck("cache directory", fmt.Sprintf("%s writable, but probe close failed: %v", cacheDir, err))
	}
	if err := os.Remove(probePath); err != nil {
		return warnCheck("cache directory", fmt.Sprintf("%s writable, but cleanup failed: %v", cacheDir, err))
	}
	return okCheck("cache directory", cacheDir+" writable")
}

func featureModesCheck(features config.FeatureConfig) DoctorCheck {
	detail := fmt.Sprintf(
		"image=%s avatar=%s emoji=%s emote=%s animation=%s kitty=%t",
		features.ImageMode,
		features.AvatarMode,
		features.EmojiMode,
		features.EmoteMode,
		features.AnimationMode,
		features.EnableKittyImages,
	)
	if unknown := unknownFeatureModes(features); len(unknown) > 0 {
		return warnCheck("feature modes", detail+"; unknown: "+strings.Join(unknown, ", "))
	}
	return okCheck("feature modes", detail)
}

func unknownFeatureModes(features config.FeatureConfig) []string {
	var unknown []string
	if !oneOf(features.ImageMode, "auto", "off", "small", "normal", "large") {
		unknown = append(unknown, "image="+features.ImageMode)
	}
	if !oneOf(features.AvatarMode, "off", "initials", "image") {
		unknown = append(unknown, "avatar="+features.AvatarMode)
	}
	if !oneOf(features.EmojiMode, "unicode", "image") {
		unknown = append(unknown, "emoji="+features.EmojiMode)
	}
	if !oneOf(features.EmoteMode, "text", "image") {
		unknown = append(unknown, "emote="+features.EmoteMode)
	}
	if !oneOf(features.AnimationMode, "off", "reduced", "fast", "expressive") {
		unknown = append(unknown, "animation="+features.AnimationMode)
	}
	return unknown
}

func tokenCredentialsFromConfig(cfg config.TwitchConfig) twitch.TokenCredentials {
	return twitch.TokenCredentials{
		Username:     cfg.Username,
		OAuthToken:   cfg.OAuthToken,
		RefreshToken: cfg.RefreshToken,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
	}
}

func tokenValidationDetail(validation twitch.TokenValidationResult, fallback string) string {
	if detail := strings.TrimSpace(validation.Detail); detail != "" {
		return detail
	}
	return fallback
}

func refreshAvailabilityDetail(available bool) string {
	if available {
		return "refresh credentials are available"
	}
	return "refresh credentials are unavailable"
}

func usernameOwnershipDetail(expected, actual string) string {
	if mismatch := tokenUsernameMismatch(expected, actual); mismatch != "" {
		return mismatch
	}
	return "OAuth token belongs to a different Twitch user"
}

func tokenUsernameMismatch(expected, actual string) string {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" || actual == "" || strings.EqualFold(expected, actual) {
		return ""
	}
	return fmt.Sprintf("OAuth token belongs to Twitch user %q, not configured username %q", actual, expected)
}

func tokenScopesCSV(scopes []twitch.TokenScope) string {
	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, string(scope))
	}
	return strings.Join(values, ", ")
}

func redactSensitive(detail string, cfg config.Config) string {
	detail = oauthPattern.ReplaceAllString(detail, "[redacted]")
	for _, secret := range sensitiveValues(cfg) {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			detail = strings.ReplaceAll(detail, secret, "[redacted]")
		}
	}
	return detail
}

func sensitiveValues(cfg config.Config) []string {
	values := []string{cfg.Twitch.OAuthToken, cfg.Twitch.RefreshToken, cfg.Twitch.ClientSecret}
	token := strings.TrimSpace(cfg.Twitch.OAuthToken)
	if prefix, body, ok := strings.Cut(token, ":"); ok && strings.EqualFold(prefix, "oauth") {
		values = append(values, body)
	}
	return values
}

func envMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}

func okCheck(name, detail string) DoctorCheck {
	return DoctorCheck{Name: name, Status: DoctorStatusOK, Detail: detail}
}

func warnCheck(name, detail string) DoctorCheck {
	return DoctorCheck{Name: name, Status: DoctorStatusWarn, Detail: detail}
}
