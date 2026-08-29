package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
	"github.com/yang-bin-free/claude-phone/pkg/engine"
	"github.com/yang-bin-free/claude-phone/pkg/product"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "autostart" {
		runAutostart(os.Args[2:])
		return
	}
	fs := flag.NewFlagSet("codeafar", flag.ExitOnError)
	desktopAddr := fs.String("desktop-addr", "127.0.0.1:9877", "loopback desktop listen address")
	claudeBin := fs.String("claude-bin", "claude", "Claude CLI binary")
	codexBin := fs.String("codex-bin", "codex", "Codex CLI binary")
	workdir := fs.String("workdir", ".", "default Claude working directory")
	permission := fs.String("permission", "default", "default Claude permission mode")
	dataDir := fs.String("data-dir", "", "CodeAfar configuration directory")
	_ = fs.Parse(os.Args[1:])

	if err := validateDesktopAddr(*desktopAddr); err != nil {
		log.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	resolvedDataDir, _, err := product.ResolveDataDir(home, *dataDir)
	if err != nil {
		log.Fatalf("CodeAfar data migration failed: %v", err)
	}
	// Explicit flags win over config.yaml, which wins over the defaults.
	claudeCommand := *claudeBin
	codexCommand := *codexBin
	claudeFlagSet := false
	codexFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "claude-bin":
			claudeFlagSet = true
		case "codex-bin":
			codexFlagSet = true
		}
	})
	if !claudeFlagSet || !codexFlagSet {
		fileClaude, fileCodex := engine.ReadPersistedCommands(resolvedDataDir)
		if !claudeFlagSet && fileClaude != "" {
			claudeCommand = fileClaude
		}
		if !codexFlagSet && fileCodex != "" {
			codexCommand = fileCodex
		}
	}
	token, err := generateAdminToken()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := newApplication(ctx, appConfig{
		DesktopAddr: *desktopAddr, ClaudeBin: claudeCommand, CodexBin: codexCommand, DefaultWorkdir: *workdir,
		DefaultPermission: *permission, DataDir: resolvedDataDir, AdminToken: token,
	}, appDependencies{})
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
	defer app.Close()

	pageURL, err := desktop.URLWithAdminToken(app.BaseURL(), token)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("CodeAfar desktop service listening on %s", app.BaseURL())
	commands := newNativeCommands(app, stop)
	if err := desktop.RunNative(ctx, pageURL, commands); err != nil {
		stop()
		log.Print(err)
	}
	stop()
}

func newNativeCommands(app *application, stop context.CancelFunc) desktop.Commands {
	return desktop.Commands{
		States: app.MenuStates(),
		Pause:  app.Pause,
		Resume: app.Resume,
		ToggleAutostart: func() error {
			if desktop.AutostartEnabled() {
				return desktop.UninstallAutostart()
			}
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			return desktop.InstallAutostart(executable, nil)
		},
		Quit: func() {
			_ = app.Close()
			stop()
		},
	}
}

func runAutostart(args []string) {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "install":
		executable, err := os.Executable()
		if err != nil {
			log.Fatal(err)
		}
		if err := desktop.InstallAutostart(executable, nil); err != nil {
			log.Fatal(err)
		}
		fmt.Println("autostart installed")
	case "uninstall":
		if err := desktop.UninstallAutostart(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("autostart uninstalled")
	case "status":
		if desktop.AutostartEnabled() {
			fmt.Println("enabled")
		} else {
			fmt.Println("disabled")
		}
	default:
		log.Fatal("usage: codeafar autostart install|uninstall|status")
	}
}

func generateAdminToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func validateDesktopAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid desktop address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("desktop address must use an explicit loopback host")
	}
	return nil
}
