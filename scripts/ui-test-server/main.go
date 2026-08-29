// Command ui-test-server serves the CodeAfar desktop web UI without the
// native macOS shell. It exists so browser-driven UI tests (for example
// scripts/ui-test-cli-settings.js) can drive the real chat and admin views
// against a real engine, with an isolated data directory and a fixed admin
// token. It wires the same desktop.NewHandler + engine.New stack as
// cmd/mac-app, minus the menu bar and webview loop.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
	"github.com/yang-bin-free/claude-phone/pkg/engine"
	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

func main() {
	fs := flag.NewFlagSet("ui-test-server", flag.ExitOnError)
	addr := fs.String("desktop-addr", "127.0.0.1:9887", "loopback desktop listen address")
	dataDir := fs.String("data-dir", "", "isolated CodeAfar data directory (required)")
	workdir := fs.String("workdir", ".", "default working directory")
	claudeBin := fs.String("claude-bin", "claude", "Claude CLI command")
	codexBin := fs.String("codex-bin", "codex", "Codex CLI command")
	adminToken := fs.String("admin-token", "ui-test-admin-token", "fixed admin token embedded in the page URL")
	_ = fs.Parse(os.Args[1:])

	if *dataDir == "" {
		log.Fatal("-data-dir is required")
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// Same startup precedence as cmd/mac-app: explicit flags win over the
	// persisted commands in config.yaml, which win over the defaults. An
	// explicit flag is one actually present on the command line.
	claudeCommandRequested := *claudeBin
	codexCommandRequested := *codexBin
	claudeFlagSet, codexFlagSet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "claude-bin":
			claudeFlagSet = true
		case "codex-bin":
			codexFlagSet = true
		}
	})
	if !claudeFlagSet || !codexFlagSet {
		fileClaude, fileCodex := engine.ReadPersistedCommands(*dataDir)
		if !claudeFlagSet && fileClaude != "" {
			claudeCommandRequested = fileClaude
		}
		if !codexFlagSet && fileCodex != "" {
			codexCommandRequested = fileCodex
		}
	}

	// Resolve providers the same way cmd/mac-app does: keep the app usable
	// when one CLI is missing, and record the reason for the UI.
	claudeCommand, claudeVersion, claudeErr := resolveProvider(claudeCommandRequested, "Claude", desktop.ResolveClaudeBinary)
	codexCommand, codexVersion, codexErr := resolveProvider(codexCommandRequested, "Codex", desktop.ResolveCodexBinary)
	desktopDeviceToken := "desktop-" + *adminToken
	e := engine.New(engine.Config{
		Addr:                    listener.Addr().String(),
		ClaudeBin:               claudeCommand.Path,
		ClaudeBinArgs:           claudeCommand.PrependArgs,
		ClaudeVersion:           claudeVersion,
		ClaudeUnavailableReason: errorText(claudeErr),
		CodexBin:                codexCommand.Path,
		CodexBinArgs:            codexCommand.PrependArgs,
		CodexVersion:            codexVersion,
		CodexUnavailableReason:  errorText(codexErr),
		DefaultWorkingDir:       *workdir,
		DefaultPermission:       "default",
		DataDir:                 *dataDir,
		DeviceTokens:            map[string]string{desktopDeviceToken: "Mac"},
		DesktopDeviceToken:      desktopDeviceToken,
	})

	status := func() desktop.AppStatus {
		if claudeErr != nil && codexErr != nil {
			return desktop.AppStatus{Error: errorText(claudeErr)}
		}
		return desktop.AppStatus{Ready: true, ClaudeBin: claudeCommand.String(), ClaudeVersion: claudeVersion, CodexBin: codexCommand.String(), CodexVersion: codexVersion}
	}
	handler := desktop.NewHandler(desktop.HandlerOptions{
		EngineHandler: func() http.Handler { return e.Handler() },
		AdminHandler:  func() http.Handler { return e.AdminHandler(*adminToken) },
		Status:        status,
		AddProject: func(path string) (protocol.ProjectInfo, error) {
			return e.AddProject(path)
		},
		AdminToken: *adminToken,
	})

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = e.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	pageURL, err := desktop.URLWithAdminToken("http://"+listener.Addr().String()+"/", *adminToken)
	if err != nil {
		log.Fatalf("build page url: %v", err)
	}
	// Single line a test script can wait for.
	fmt.Printf("UI_TEST_READY %s\n", pageURL)
	if err := server.Serve(listener); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func resolveProvider(requested, name string, resolve func(string) (desktop.ResolvedCommand, error)) (desktop.ResolvedCommand, string, error) {
	command, err := resolve(requested)
	if err != nil {
		return desktop.ResolvedCommand{}, "", err
	}
	version, err := engine.DetectCLIVersion(command.Args(), name)
	if err != nil {
		return desktop.ResolvedCommand{}, "", fmt.Errorf("%s CLI check failed: %w", name, err)
	}
	return command, version, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
