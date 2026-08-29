// Package engine exposes the headless Claude Phone engine over WebSocket.
package engine

import (
	"os"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/product"
)

const DefaultAddr = "127.0.0.1:9876"

type Config struct {
	Addr                    string
	AgentVersion            string
	ClaudeVersion           string
	ClaudeBin               string
	ClaudeBinArgs           []string
	ClaudeUnavailableReason string
	CodexVersion            string
	CodexBin                string
	CodexBinArgs            []string
	CodexUnavailableReason  string
	DefaultWorkingDir       string
	DefaultPermission       string
	MaxConcurrentSession    int
	WriteTimeout            time.Duration
	DeviceTokens            map[string]string
	DesktopDeviceToken      string
	DataDir                 string
	ConfigPollInterval      time.Duration
	HealthPollInterval      time.Duration
	StalledAfter            time.Duration
	UnresponsiveAfter       time.Duration
}

func (c Config) withDefaults() Config {
	if c.Addr == "" {
		c.Addr = DefaultAddr
	}
	if c.AgentVersion == "" {
		c.AgentVersion = "0.1.0-dev"
	}
	if c.ClaudeVersion == "" {
		c.ClaudeVersion = "unknown"
	}
	if c.ClaudeBin == "" {
		c.ClaudeBin = "claude"
	}
	if c.CodexVersion == "" {
		c.CodexVersion = "unknown"
	}
	if c.CodexBin == "" {
		c.CodexBin = "codex"
	}
	if c.DefaultWorkingDir == "" {
		c.DefaultWorkingDir = "."
	}
	if c.DefaultPermission == "" {
		c.DefaultPermission = "default"
	}
	if c.MaxConcurrentSession <= 0 {
		c.MaxConcurrentSession = 5
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 5 * time.Second
	}
	if c.HealthPollInterval <= 0 {
		c.HealthPollInterval = 30 * time.Second
	}
	if c.StalledAfter <= 0 {
		c.StalledAfter = 2 * time.Minute
	}
	if c.UnresponsiveAfter <= 0 {
		c.UnresponsiveAfter = 5 * time.Minute
	}
	if c.DataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.DataDir = product.DefaultDataDir(home)
		} else {
			c.DataDir = product.DataDirName
		}
	}
	if c.DeviceTokens == nil {
		c.DeviceTokens = map[string]string{}
	}
	return c
}
