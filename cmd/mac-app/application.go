package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/desktop"
	"github.com/yang-bin-free/claude-phone/pkg/engine"
	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

type managedEngine interface {
	Handler() http.Handler
	AdminHandler(string) http.Handler
	Status() engine.StatusReport
	AddProject(string) (protocol.ProjectInfo, error)
	Close() error
}

type appConfig struct {
	DesktopAddr       string
	ClaudeBin         string
	CodexBin          string
	DefaultWorkdir    string
	DefaultPermission string
	DataDir           string
	AdminToken        string
}

type appDependencies struct {
	resolveClaude func(string) (desktop.ResolvedCommand, error)
	resolveCodex  func(string) (desktop.ResolvedCommand, error)
	detectVersion func([]string, string) (string, error)
	newEngine     func(engine.Config) managedEngine
	listen        func(string, string) (net.Listener, error)
	serve         func(context.Context, net.Listener, http.Handler) error
}

type application struct {
	cfg  appConfig
	deps appDependencies

	ctx    context.Context
	cancel context.CancelFunc

	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	engine      managedEngine
	status      desktop.AppStatus
	handler     http.Handler
	listener    net.Listener
	serverDone  chan error
	menuStates  chan desktop.MenuState
	closeOnce   sync.Once
	closeErr    error
}

func newApplication(parent context.Context, cfg appConfig, deps appDependencies) *application {
	if deps.resolveClaude == nil {
		deps.resolveClaude = desktop.ResolveClaudeBinary
	}
	if deps.resolveCodex == nil {
		deps.resolveCodex = desktop.ResolveCodexBinary
	}
	if deps.detectVersion == nil {
		deps.detectVersion = engine.DetectCLIVersion
	}
	if deps.newEngine == nil {
		deps.newEngine = func(cfg engine.Config) managedEngine { return engine.New(cfg) }
	}
	if deps.listen == nil {
		deps.listen = net.Listen
	}
	if deps.serve == nil {
		deps.serve = desktop.Serve
	}
	ctx, cancel := context.WithCancel(parent)
	app := &application{cfg: cfg, deps: deps, ctx: ctx, cancel: cancel, serverDone: make(chan error, 1), menuStates: make(chan desktop.MenuState, 1)}
	app.handler = desktop.NewHandler(desktop.HandlerOptions{
		EngineHandler: app.engineHandler,
		AdminHandler:  app.adminHandler,
		Status:        app.Status,
		AddProject:    app.addProject,
		AdminToken:    cfg.AdminToken,
	})
	return app
}

func (a *application) Start() error {
	a.lifecycleMu.Lock()
	if a.listener != nil {
		a.lifecycleMu.Unlock()
		return errors.New("desktop application already started")
	}
	listener, err := a.deps.listen("tcp", a.cfg.DesktopAddr)
	if err != nil {
		a.lifecycleMu.Unlock()
		return fmt.Errorf("listen for desktop UI: %w", err)
	}
	a.listener = listener
	a.lifecycleMu.Unlock()

	go func() { a.serverDone <- a.deps.serve(a.ctx, listener, a.handler) }()
	_ = a.Resume()
	go a.publishMenuState()
	return nil
}

func (a *application) Resume() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	a.mu.RLock()
	alreadyReady := a.engine != nil
	a.mu.RUnlock()
	if alreadyReady {
		return nil
	}

	claudeCommand, claudeVersion, claudeErr := a.resolveProvider(a.cfg.ClaudeBin, "Claude", a.deps.resolveClaude)
	codexCommand, codexVersion, codexErr := a.resolveProvider(a.cfg.CodexBin, "Codex", a.deps.resolveCodex)
	if claudeErr != nil && codexErr != nil {
		err := errors.Join(claudeErr, codexErr)
		a.setUnavailable(err, false)
		a.emitMenuState()
		return err
	}
	desktopDeviceToken := "desktop-" + a.cfg.AdminToken
	instance := a.deps.newEngine(engine.Config{
		Addr:                    a.cfg.DesktopAddr,
		ClaudeBin:               firstNonEmpty(claudeCommand.Path, a.cfg.ClaudeBin, "claude"),
		ClaudeBinArgs:           claudeCommand.PrependArgs,
		ClaudeVersion:           firstNonEmpty(claudeVersion, "unknown"),
		ClaudeUnavailableReason: errorText(claudeErr),
		CodexBin:                firstNonEmpty(codexCommand.Path, a.cfg.CodexBin, "codex"),
		CodexBinArgs:            codexCommand.PrependArgs,
		CodexVersion:            firstNonEmpty(codexVersion, "unknown"),
		CodexUnavailableReason:  errorText(codexErr),
		DefaultWorkingDir:       a.cfg.DefaultWorkdir,
		DefaultPermission:       a.cfg.DefaultPermission,
		DataDir:                 a.cfg.DataDir,
		DeviceTokens:            map[string]string{desktopDeviceToken: "Mac"},
		DesktopDeviceToken:      desktopDeviceToken,
	})
	a.mu.Lock()
	a.engine = instance
	a.status = desktop.AppStatus{Ready: true, ClaudeBin: claudeCommand.String(), ClaudeVersion: claudeVersion, CodexBin: codexCommand.String(), CodexVersion: codexVersion}
	a.mu.Unlock()
	a.emitMenuState()
	return nil
}

func (a *application) resolveProvider(requested, name string, resolve func(string) (desktop.ResolvedCommand, error)) (desktop.ResolvedCommand, string, error) {
	command, err := resolve(requested)
	if err != nil {
		return desktop.ResolvedCommand{}, "", err
	}
	version, err := a.deps.detectVersion(command.Args(), name)
	if err != nil {
		return desktop.ResolvedCommand{}, "", fmt.Errorf("%s CLI check failed: %w", name, err)
	}
	return command, version, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *application) Pause() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.mu.Lock()
	instance := a.engine
	a.engine = nil
	a.status = desktop.AppStatus{Paused: true}
	a.mu.Unlock()
	if instance != nil {
		err := instance.Close()
		a.emitMenuState()
		return err
	}
	a.emitMenuState()
	return nil
}

func (a *application) Close() error {
	a.closeOnce.Do(func() {
		a.cancel()
		a.mu.Lock()
		instance := a.engine
		a.engine = nil
		a.mu.Unlock()
		if instance != nil {
			a.closeErr = instance.Close()
		}
		if a.listener != nil {
			if err := <-a.serverDone; a.closeErr == nil {
				a.closeErr = err
			}
		}
	})
	return a.closeErr
}

func (a *application) Handler() http.Handler { return a.handler }

func (a *application) MenuStates() <-chan desktop.MenuState { return a.menuStates }

func (a *application) BaseURL() string {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.listener == nil {
		return ""
	}
	return "http://" + a.listener.Addr().String() + "/"
}

func (a *application) Status() desktop.AppStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *application) EngineStatus() engine.StatusReport {
	a.mu.RLock()
	instance := a.engine
	a.mu.RUnlock()
	if instance == nil {
		return engine.StatusReport{}
	}
	return instance.Status()
}

func (a *application) engineHandler() http.Handler {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.engine == nil {
		return nil
	}
	return a.engine.Handler()
}

func (a *application) adminHandler() http.Handler {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.engine == nil {
		return nil
	}
	return a.engine.AdminHandler(a.cfg.AdminToken)
}

func (a *application) addProject(path string) (protocol.ProjectInfo, error) {
	a.mu.RLock()
	instance := a.engine
	a.mu.RUnlock()
	if instance == nil {
		return protocol.ProjectInfo{}, errors.New("desktop engine unavailable")
	}
	return instance.AddProject(path)
}

func (a *application) setUnavailable(err error, paused bool) {
	a.mu.Lock()
	a.status = desktop.AppStatus{Paused: paused, Error: err.Error()}
	a.mu.Unlock()
}

func (a *application) publishMenuState() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	a.emitMenuState()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.emitMenuState()
		}
	}
}

func (a *application) emitMenuState() {
	status := a.Status()
	state := desktop.MenuState{
		Ready: status.Ready, Paused: status.Paused, StatusText: status.Error,
		Autostart: desktop.AutostartEnabled(),
	}
	if status.Ready {
		report := a.EngineStatus()
		state.Devices = len(report.ConnectedDevices)
		state.Sessions = len(report.Sessions)
	}
	select {
	case a.menuStates <- state:
	default:
		select {
		case <-a.menuStates:
		default:
		}
		select {
		case a.menuStates <- state:
		default:
		}
	}
}
