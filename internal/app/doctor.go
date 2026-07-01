package app

import (
	"os"

	"github.com/w0rxbend/twi/internal/config"
)

type DoctorReport struct {
	Checks []DoctorCheck
}

type DoctorCheck struct {
	Name   string
	OK     bool
	Detail string
}

func Doctor(cfg config.Config) DoctorReport {
	checks := []DoctorCheck{
		{Name: "config path", OK: cfg.Path != "", Detail: cfg.Path},
		{Name: "username", OK: cfg.Twitch.Username != "", Detail: present(cfg.Twitch.Username)},
		{Name: "oauth token", OK: cfg.Twitch.OAuthToken != "", Detail: present(cfg.Twitch.OAuthToken)},
		{Name: "channels", OK: len(cfg.DefaultChannels) > 0, Detail: channelDetail(cfg.DefaultChannels)},
		{Name: "terminal", OK: os.Getenv("TERM") != "", Detail: envDetail("TERM")},
		{Name: "kitty images", OK: cfg.Features.EnableKittyImages, Detail: cfg.Features.ImageMode},
		{Name: "animation", OK: cfg.Features.AnimationMode != "off", Detail: cfg.Features.AnimationMode},
	}
	return DoctorReport{Checks: checks}
}

func present(value string) string {
	if value == "" {
		return "missing"
	}
	return "present"
}

func envDetail(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return "missing"
}

func channelDetail(channels []string) string {
	if len(channels) == 0 {
		return "none configured"
	}
	return channels[0]
}
