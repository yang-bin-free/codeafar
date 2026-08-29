package engine

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

func (e *Engine) updateRuntimeConfig(next runtimeConfig) error {
	info, err := os.Stat(next.DefaultWorkingDir)
	if err != nil || !info.IsDir() || !filepath.IsAbs(next.DefaultWorkingDir) {
		return errors.New("default working directory must be an existing absolute directory")
	}
	switch next.DefaultPermission {
	case "default", "acceptEdits", "bypassPermissions":
	default:
		return errors.New("invalid default permission")
	}
	if next.MaxConcurrentSessions < 1 || next.MaxConcurrentSessions > 20 {
		return errors.New("max concurrent sessions must be between 1 and 20")
	}
	if err := os.MkdirAll(e.cfg.DataDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(e.cfg.DataDir, "config.yaml")
	temp, err := os.CreateTemp(e.cfg.DataDir, "config-*.yaml")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	err = yaml.NewEncoder(temp).Encode(next)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	return e.reloadRuntimeConfig()
}

type runtimeConfig struct {
	DefaultWorkingDir     string `yaml:"defaultWorkingDir"`
	DefaultPermission     string `yaml:"defaultPermission"`
	MaxConcurrentSessions int    `yaml:"maxConcurrentSessions"`
	ClaudeCommand         string `yaml:"claudeCommand,omitempty"`
	CodexCommand          string `yaml:"codexCommand,omitempty"`
}

func (e *Engine) runtimeConfig() runtimeConfig {
	e.configMu.RLock()
	defer e.configMu.RUnlock()
	return e.runtime
}

func (e *Engine) reloadRuntimeConfig() error {
	b, err := os.ReadFile(filepath.Join(e.cfg.DataDir, "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var next runtimeConfig
	if err := yaml.Unmarshal(b, &next); err != nil {
		return err
	}
	current := e.runtimeConfig()
	if next.DefaultWorkingDir == "" {
		next.DefaultWorkingDir = current.DefaultWorkingDir
	}
	if next.DefaultPermission == "" {
		next.DefaultPermission = current.DefaultPermission
	}
	if next.MaxConcurrentSessions <= 0 {
		next.MaxConcurrentSessions = current.MaxConcurrentSessions
	}
	e.configMu.Lock()
	e.runtime = next
	e.configMu.Unlock()
	e.manager.SetMaxConcurrent(next.MaxConcurrentSessions)
	return nil
}

func (e *Engine) watchRuntimeConfig() {
	interval := e.cfg.ConfigPollInterval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = e.reloadRuntimeConfig()
		case <-e.stopWatch:
			return
		}
	}
}
