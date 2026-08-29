package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
	"github.com/yang-bin-free/claude-phone/pkg/engine"
	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

type fakeManagedEngine struct {
	closed       atomic.Bool
	addedProject string
}

func (e *fakeManagedEngine) AddProject(path string) (protocol.ProjectInfo, error) {
	e.addedProject = path
	return protocol.ProjectInfo{Name: "Demo", Path: path, Permission: "default"}, nil
}

func (e *fakeManagedEngine) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusSwitchingProtocols) })
}

func (e *fakeManagedEngine) AdminHandler(string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func (e *fakeManagedEngine) Close() error {
	e.closed.Store(true)
	return nil
}

func (e *fakeManagedEngine) Status() engine.StatusReport {
	return engine.StatusReport{ConnectedDevices: []string{"Mac"}, Sessions: []engine.SessionSnapshot{{SessionID: "s1"}}}
}

func TestApplicationKeepsDesktopAliveWhenClaudeIsUnavailable(t *testing.T) {
	app := newApplication(context.Background(), appConfig{DesktopAddr: "127.0.0.1:0", ClaudeBin: "claude", AdminToken: "token"}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{}, errors.New("claude missing")
		},
		resolveCodex: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{}, errors.New("codex missing")
		},
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	status := app.Status()
	if status.Ready || !strings.Contains(status.Error, "claude missing") {
		t.Fatalf("status=%+v", status)
	}
	assertAppHTTPStatus(t, app.Handler(), "/", http.StatusOK)
	assertAppHTTPStatus(t, app.Handler(), "/desktop/status", http.StatusOK)
	assertAppHTTPStatus(t, app.Handler(), "/ws", http.StatusServiceUnavailable)
}

func TestApplicationStartsWhenOnlyCodexIsAvailable(t *testing.T) {
	var captured engine.Config
	app := newApplication(context.Background(), appConfig{
		DesktopAddr: "127.0.0.1:0", ClaudeBin: "claude", CodexBin: "codex", AdminToken: "token",
	}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{}, errors.New("claude missing")
		},
		resolveCodex: func(string) (desktop.ResolvedCommand, error) { return desktop.ResolvedCommand{Path: "/tmp/codex"}, nil },
		detectVersion: func(command []string, _ string) (string, error) {
			if len(command) != 1 || command[0] != "/tmp/codex" {
				t.Fatalf("version command=%v", command)
			}
			return "0.144.1", nil
		},
		newEngine: func(cfg engine.Config) managedEngine {
			captured = cfg
			return &fakeManagedEngine{}
		},
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	status := app.Status()
	if !status.Ready || status.CodexBin != "/tmp/codex" || status.CodexVersion != "0.144.1" {
		t.Fatalf("status=%+v", status)
	}
	if captured.CodexBin != "/tmp/codex" || captured.CodexVersion != "0.144.1" || captured.ClaudeUnavailableReason != "claude missing" {
		t.Fatalf("engine config=%+v", captured)
	}
	if len(captured.CodexBinArgs) != 0 {
		t.Fatalf("engine config codex args=%v", captured.CodexBinArgs)
	}
}

func TestStartUsesConfigFileCommandWhenFlagUnset(t *testing.T) {
	var captured engine.Config
	app := newApplication(context.Background(), appConfig{
		DesktopAddr: "127.0.0.1:0", ClaudeBin: "wrapper claude", CodexBin: "codex", AdminToken: "token",
	}, appDependencies{
		resolveClaude: func(requested string) (desktop.ResolvedCommand, error) {
			if requested != "wrapper claude" {
				t.Fatalf("requested=%q", requested)
			}
			return desktop.ResolvedCommand{Path: "/bin/wrapper", PrependArgs: []string{"claude"}}, nil
		},
		resolveCodex: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{}, errors.New("codex missing")
		},
		detectVersion: func(command []string, _ string) (string, error) {
			if len(command) > 0 && command[0] == "/bin/wrapper" {
				return "2.1.245", nil
			}
			return "", errors.New("no version")
		},
		newEngine: func(cfg engine.Config) managedEngine {
			captured = cfg
			return &fakeManagedEngine{}
		},
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.Status().Ready {
		t.Fatalf("status=%+v", app.Status())
	}
	if captured.ClaudeBin != "/bin/wrapper" || len(captured.ClaudeBinArgs) != 1 || captured.ClaudeBinArgs[0] != "claude" {
		t.Fatalf("engine config claude bin=%q args=%v", captured.ClaudeBin, captured.ClaudeBinArgs)
	}
	if captured.CodexUnavailableReason != "codex missing" {
		t.Fatalf("engine config=%+v", captured)
	}
}

func TestApplicationPauseAndResumeKeepsDesktopHandler(t *testing.T) {
	created := make([]*fakeManagedEngine, 0, 2)
	configs := make([]engine.Config, 0, 2)
	app := newApplication(context.Background(), appConfig{DesktopAddr: "127.0.0.1:0", ClaudeBin: "claude", AdminToken: "token"}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{Path: "/tmp/claude"}, nil
		},
		detectVersion: func([]string, string) (string, error) { return "1.2.3", nil },
		newEngine: func(cfg engine.Config) managedEngine {
			instance := &fakeManagedEngine{}
			created = append(created, instance)
			configs = append(configs, cfg)
			return instance
		},
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.Status().Ready {
		t.Fatalf("status=%+v", app.Status())
	}
	if len(configs) != 1 || configs[0].DesktopDeviceToken != "desktop-token" || configs[0].DeviceTokens["desktop-token"] != "Mac" {
		t.Fatalf("desktop engine config=%+v", configs)
	}
	assertAppHTTPStatus(t, app.Handler(), "/ws", http.StatusSwitchingProtocols)

	if err := app.Pause(); err != nil {
		t.Fatal(err)
	}
	if !created[0].closed.Load() || !app.Status().Paused {
		t.Fatalf("closed=%v status=%+v", created[0].closed.Load(), app.Status())
	}
	assertAppHTTPStatus(t, app.Handler(), "/ws", http.StatusServiceUnavailable)
	assertAppHTTPStatus(t, app.Handler(), "/", http.StatusOK)

	if err := app.Resume(); err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 || len(configs) != 2 || configs[1].DesktopDeviceToken != "desktop-token" || !app.Status().Ready {
		t.Fatalf("created=%d status=%+v", len(created), app.Status())
	}
}

func TestApplicationDesktopProjectEndpointUsesCurrentEngine(t *testing.T) {
	instance := &fakeManagedEngine{}
	app := newApplication(context.Background(), appConfig{DesktopAddr: "127.0.0.1:0", ClaudeBin: "claude", AdminToken: "token"}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{Path: "/tmp/claude"}, nil
		},
		detectVersion: func([]string, string) (string, error) { return "1.2.3", nil },
		newEngine:     func(engine.Config) managedEngine { return instance },
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	directory := t.TempDir()
	request := httptest.NewRequest(http.MethodPost, "/desktop/projects", strings.NewReader(`{"path":`+strconv.Quote(directory)+`}`))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CodeAfar-Admin-Token", "token")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusCreated || instance.addedProject != directory {
		t.Fatalf("status=%d added=%q body=%s", response.Code, instance.addedProject, response.Body.String())
	}
}

func TestApplicationCloseIsIdempotent(t *testing.T) {
	app := newApplication(context.Background(), appConfig{DesktopAddr: "127.0.0.1:0", AdminToken: "token"}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) { return desktop.ResolvedCommand{}, errors.New("missing") },
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeQuitClosesEngineAndCancelsContext(t *testing.T) {
	instance := &fakeManagedEngine{}
	app := newApplication(context.Background(), appConfig{DesktopAddr: "127.0.0.1:0", ClaudeBin: "claude", AdminToken: "token"}, appDependencies{
		resolveClaude: func(string) (desktop.ResolvedCommand, error) {
			return desktop.ResolvedCommand{Path: "/tmp/claude"}, nil
		},
		detectVersion: func([]string, string) (string, error) { return "1.2.3", nil },
		newEngine:     func(engine.Config) managedEngine { return instance },
	})
	if err := app.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	commands := newNativeCommands(app, cancel)
	commands.Quit()
	if !instance.closed.Load() {
		t.Fatal("native quit returned before the engine closed")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("native quit did not cancel the application context")
	}
}

func assertAppHTTPStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != want {
		t.Fatalf("%s status=%d want=%d body=%s", path, w.Code, want, w.Body.String())
	}
}

var _ managedEngine = (*fakeManagedEngine)(nil)
var _ = desktop.AppStatus{}
