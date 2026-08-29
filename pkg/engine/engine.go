package engine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/protocol"
	"github.com/yang-bin-free/claude-phone/pkg/provider"
	"github.com/yang-bin-free/claude-phone/pkg/session"
)

type claudeProc = provider.Process

type ClaudeFactory func(session.ClaudeConfig) claudeProc

type Engine struct {
	cfg           Config
	manager       *session.Manager
	providers     *provider.Registry
	sessionExists func(string, string) bool
	server        *http.Server

	mu                 sync.RWMutex
	resumeMu           sync.Mutex
	createMu           sync.Mutex
	textMu             sync.Mutex
	permissionMu       sync.Mutex
	clients            map[string]*client
	procs              map[string]claudeProc
	projects           *projectStore
	devices            *deviceStore
	history            *historyStore
	permissions        *permissionStore
	templates          *templateStore
	startedAt          time.Time
	configMu           sync.RWMutex
	runtime            runtimeConfig
	stopWatch          chan struct{}
	closeOnce          sync.Once
	power              powerInhibitor
	activity           map[string]time.Time
	healthState        map[string]string
	queues             map[string][]queuedPrompt
	busy               map[string]bool
	queueSeq           uint64
	createRequests     map[string]createResult
	textRequests       map[string][sha256.Size]byte
	textRequestOrder   map[string][]string
	pendingPermissions map[string]string
	updateSession      func(*session.Session) error
}

type createResult struct {
	message   protocol.SessionCreatedMsg
	signature string
}

func New(cfg Config) *Engine {
	cfg = cfg.withDefaults()
	e := &Engine{
		cfg:                cfg,
		manager:            session.NewManager(session.ManagerConfig{MaxConcurrent: cfg.MaxConcurrentSession}),
		clients:            map[string]*client{},
		procs:              map[string]claudeProc{},
		projects:           newProjectStore(cfg.DataDir),
		devices:            newDeviceStore(cfg.DataDir),
		history:            newHistoryStore(cfg.DataDir),
		permissions:        newPermissionStore(cfg.DataDir),
		templates:          newTemplateStore(cfg.DataDir),
		startedAt:          time.Now(),
		runtime:            runtimeConfig{DefaultWorkingDir: cfg.DefaultWorkingDir, DefaultPermission: cfg.DefaultPermission, MaxConcurrentSessions: cfg.MaxConcurrentSession},
		sessionExists:      session.ClaudeSessionExists,
		stopWatch:          make(chan struct{}),
		power:              newPowerInhibitor(),
		activity:           map[string]time.Time{},
		healthState:        map[string]string{},
		queues:             map[string][]queuedPrompt{},
		busy:               map[string]bool{},
		createRequests:     map[string]createResult{},
		textRequests:       map[string][sha256.Size]byte{},
		textRequestOrder:   map[string][]string{},
		pendingPermissions: map[string]string{},
	}
	e.providers = provider.NewRegistry(
		provider.NewClaudeAdapterWithAvailability(cfg.ClaudeBin, cfg.ClaudeBinArgs, cfg.ClaudeUnavailableReason == "", cfg.ClaudeUnavailableReason),
		provider.NewCodexAdapter(cfg.CodexBin, cfg.CodexBinArgs, cfg.CodexUnavailableReason == "", cfg.CodexUnavailableReason),
	)
	e.updateSession = e.history.UpdateSession
	if persisted, err := e.history.Restore(); err == nil {
		for _, sess := range persisted {
			sess.SetSender(e.send)
			sess.Unsubscribe(sess.Owner)
			e.manager.Restore(sess)
		}
	}
	_ = e.reloadRuntimeConfig()
	go e.watchRuntimeConfig()
	go e.monitorHealth()
	return e
}

func (e *Engine) SetClaudeFactory(factory ClaudeFactory) {
	if factory != nil {
		e.providers = provider.NewRegistry(claudeFactoryAdapter{factory: factory})
	}
}

func (e *Engine) SetProviderRegistry(registry *provider.Registry) {
	if registry != nil {
		e.providers = registry
	}
}

type claudeFactoryAdapter struct{ factory ClaudeFactory }

func (a claudeFactoryAdapter) Descriptor() provider.Descriptor {
	return provider.NewClaudeAdapter("claude", nil).Descriptor()
}

func (a claudeFactoryAdapter) NewProcess(c provider.SessionConfig) provider.Process {
	return a.factory(session.ClaudeConfig{
		Cwd: c.Cwd, SessionID: c.SessionID, Permission: c.Permission, AddDirs: c.AddDirs,
		Resume: c.Resume, AllowedTools: c.AllowedTools, BinArgs: c.BinArgs,
	})
}

func (e *Engine) Manager() *session.Manager {
	return e.manager
}

func (e *Engine) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", e.HandleWS)
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e.Status())
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func (e *Engine) ListenAndServe() error {
	e.server = &http.Server{Addr: e.cfg.Addr, Handler: e.Handler()}
	err := e.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (e *Engine) Serve(ln net.Listener) error {
	e.server = &http.Server{Handler: e.Handler()}
	err := e.server.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (e *Engine) Close() error {
	e.closeOnce.Do(func() { close(e.stopWatch) })
	e.mu.RLock()
	clients := make([]*client, 0, len(e.clients))
	for _, cl := range e.clients {
		clients = append(clients, cl)
	}
	e.mu.RUnlock()
	for _, cl := range clients {
		_ = cl.conn.Close()
	}
	e.mu.Lock()
	for _, proc := range e.procs {
		_ = proc.Stop()
	}
	e.mu.Unlock()
	_ = e.power.Release()
	if e.server != nil {
		return e.server.Close()
	}
	return nil
}
