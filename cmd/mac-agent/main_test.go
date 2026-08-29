package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
	"github.com/yang-bin-free/claude-phone/pkg/engine"
)

func TestGenerateDeviceCredential(t *testing.T) {
	credential, err := engine.GenerateDeviceCredential(t.TempDir(), "Pixel")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if !strings.HasPrefix(credential.DeviceToken, "dt_") {
		t.Fatalf("key prefix mismatch: %q", credential.DeviceToken)
	}
	if credential.Device.Name != "Pixel" {
		t.Fatalf("name=%q", credential.Device.Name)
	}
}

func TestParseServeConfig_LocalByDefault(t *testing.T) {
	cfg, network, err := parseServeConfig([]string{"--addr", "127.0.0.1:9999", "--workdir", "/work", "--codex-bin", "/tools/codex"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9999" || cfg.DefaultWorkingDir != "/work" {
		t.Fatalf("engine config: %+v", cfg)
	}
	if cfg.CodexBin != "/tools/codex" {
		t.Fatalf("codex bin=%q", cfg.CodexBin)
	}
	if network.Enabled() {
		t.Fatalf("tsnet unexpectedly enabled: %+v", network)
	}
}

func TestResolveServeProvidersAllowsCodexOnly(t *testing.T) {
	cfg := engine.Config{ClaudeBin: "claude", CodexBin: "codex"}
	err := resolveServeProviders(&cfg,
		func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{}, errors.New("Claude missing")
		},
		func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{Path: "/tools/codex"}, nil
		},
		func(command []string, name string) (string, error) {
			if len(command) != 1 || command[0] != "/tools/codex" || name != "Codex" {
				t.Fatalf("version lookup: command=%v name=%q", command, name)
			}
			return "1.2.3", nil
		},
	)
	if err != nil {
		t.Fatalf("resolve providers: %v", err)
	}
	if cfg.CodexBin != "/tools/codex" || cfg.CodexVersion != "1.2.3" || len(cfg.CodexBinArgs) != 0 {
		t.Fatalf("codex config: %+v", cfg)
	}
	if !strings.Contains(cfg.ClaudeUnavailableReason, "Claude missing") {
		t.Fatalf("claude reason=%q", cfg.ClaudeUnavailableReason)
	}
}

func TestParseServeConfig_TSNet(t *testing.T) {
	args := []string{
		"--addr", ":9876",
		"--tsnet-dir", "/state",
		"--tsnet-hostname", "claude-mac",
		"--tsnet-auth-key", "tskey-auth-test",
		"--tsnet-control-url", "https://control.example.test",
	}
	_, got, err := parseServeConfig(args)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := tsnetOptions{
		Dir:        "/state",
		Hostname:   "claude-mac",
		AuthKey:    "tskey-auth-test",
		ControlURL: "https://control.example.test",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tsnet options: got %+v want %+v", got, want)
	}
}
