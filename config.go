package main

import (
	"os"
	"strings"
)

// Config holds all configuration options for the CLI wrapper
type Config struct {
	EnableOutputThrottling   bool
	EnableInputThrottling    bool
	EnableHeldEnterDetection bool
	ForceAnsi                bool
	LogFile                  string
	LogLevel                 string
}

// LoadConfig creates a new Config instance with values from environment variables
// Environment variables should be prefixed with CLAWDE_
func LoadConfig() *Config {
	cfg := &Config{
		// Default values
		EnableOutputThrottling:   true,
		EnableInputThrottling:    true,
		EnableHeldEnterDetection: false,
		ForceAnsi:                true,
		LogFile:                  "",
		LogLevel:                 "info",
	}

	// Override with environment variables if set
	if val := os.Getenv("CLAWDE_OUTPUT_THROTTLING"); val != "" {
		cfg.EnableOutputThrottling = parseBool(val)
	}

	if val := os.Getenv("CLAWDE_INPUT_THROTTLING"); val != "" {
		cfg.EnableInputThrottling = parseBool(val)
	}

	if val := os.Getenv("CLAWDE_HELD_ENTER_DETECTION"); val != "" {
		cfg.EnableHeldEnterDetection = parseBool(val)
	}

	if val := os.Getenv("CLAWDE_LOG_FILE"); val != "" {
		cfg.LogFile = val
	}

	if val := os.Getenv("CLAWDE_LOG_LEVEL"); val != "" {
		cfg.LogLevel = val
	}

	if val := os.Getenv("CLAWDE_FORCE_ANSI"); val != "" {
		cfg.ForceAnsi = parseBool(val)
	}

	return cfg
}

// parseBool converts string to bool, treating "true", "1", "yes", "on" as true (case-insensitive)
func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}
