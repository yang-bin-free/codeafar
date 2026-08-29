package engine

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/adminproto"
	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

const maxAdminBodyBytes = 64 << 10

func (e *Engine) AdminHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/status", func(w http.ResponseWriter, _ *http.Request) {
		projects, err := e.projects.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		templates, err := e.templates.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rc := e.runtimeConfig()
		writeAdminJSON(w, http.StatusOK, adminproto.Snapshot{
			Agent: adminStatus(e.Status(), rc.ClaudeCommand, rc.CodexCommand), Devices: e.adminDevices(), Projects: projects, Diagnostics: e.diagnostics(),
			PermissionRules: e.permissions.List(), Templates: adminTemplates(templates),
		})
	})
	mux.HandleFunc("PATCH /admin/settings", func(w http.ResponseWriter, r *http.Request) {
		var request adminproto.UpdateSettingsRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		claudeCommand, err := e.validateCommand(request.ClaudeCommand, "Claude")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		codexCommand, err := e.validateCommand(request.CodexCommand, "Codex")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := e.updateRuntimeConfig(runtimeConfig{
			DefaultWorkingDir: request.DefaultWorkingDir, DefaultPermission: request.DefaultPermission,
			MaxConcurrentSessions: request.MaxConcurrentSessions,
			ClaudeCommand:         claudeCommand, CodexCommand: codexCommand,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /admin/templates", func(w http.ResponseWriter, r *http.Request) {
		var request adminproto.Template
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		created, err := e.templates.Add(protocol.TemplateInfo{Label: request.Label, Prompt: request.Prompt})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeAdminJSON(w, http.StatusCreated, adminTemplate(created))
	})
	mux.HandleFunc("DELETE /admin/templates/{templateID}", func(w http.ResponseWriter, r *http.Request) {
		found, err := e.templates.Delete(r.PathValue("templateID"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /admin/permission-rules", func(w http.ResponseWriter, r *http.Request) {
		var rule adminproto.PermissionRule
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&rule); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		created, err := e.permissions.Add(rule.Tool, rule.Pattern)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeAdminJSON(w, http.StatusCreated, created)
	})
	mux.HandleFunc("DELETE /admin/permission-rules/{ruleID}", func(w http.ResponseWriter, r *http.Request) {
		if !e.permissions.Delete(r.PathValue("ruleID")) {
			http.Error(w, "permission rule not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /admin/projects", func(w http.ResponseWriter, r *http.Request) {
		var project adminproto.Project
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&project); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		created, err := e.projects.Add(project)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeAdminJSON(w, http.StatusCreated, created)
	})
	mux.HandleFunc("DELETE /admin/projects/{projectID}", func(w http.ResponseWriter, r *http.Request) {
		found, err := e.projects.Delete(r.PathValue("projectID"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /admin/devices/{deviceID}", func(w http.ResponseWriter, r *http.Request) {
		if !e.revokeDevice(r.PathValue("deviceID")) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /admin/devices", func(w http.ResponseWriter, r *http.Request) {
		var request adminproto.CreateDeviceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		credential, err := e.devices.Add(request.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		e.mu.Lock()
		e.cfg.DeviceTokens[credential.DeviceToken] = credential.Device.Name
		e.mu.Unlock()
		writeAdminJSON(w, http.StatusCreated, credential)
	})
	mux.HandleFunc("POST /admin/sessions/stop", func(w http.ResponseWriter, r *http.Request) {
		var request adminproto.StopSessionRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAdminBodyBytes))
		if err := decoder.Decode(&request); err != nil || request.SessionID == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		s, ok := e.manager.Get(request.SessionID)
		if !ok {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if err := e.stopSession(s); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	want := sha256.Sum256([]byte("Bearer " + token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemote(r.RemoteAddr) || !constantTimeHeaderMatch(r.Header.Get("Authorization"), want) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (e *Engine) diagnostics() adminproto.Diagnostics {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return adminproto.Diagnostics{
		GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		PID: os.Getpid(), UptimeSeconds: int64(time.Since(e.startedAt).Seconds()),
		Goroutines: runtime.NumGoroutine(), AllocBytes: memory.Alloc, SysBytes: memory.Sys,
		DataDir: e.cfg.DataDir, Caffeinating: e.power.Active(),
	}
}

func (e *Engine) adminDevices() []adminproto.DeviceSnapshot {
	records := e.devices.List()
	e.mu.RLock()
	defer e.mu.RUnlock()
	all := make(map[string]string, len(e.cfg.DeviceTokens))
	for _, record := range records {
		all[record.Token] = record.Name
	}
	for token, name := range e.cfg.DeviceTokens {
		all[token] = name
	}
	devices := make([]adminproto.DeviceSnapshot, 0, len(all))
	for token, name := range all {
		_, online := e.clients[token]
		devices = append(devices, adminproto.DeviceSnapshot{DeviceID: deviceTokenID(token), Name: name, Online: online})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	return devices
}

func (e *Engine) revokeDevice(deviceID string) bool {
	var revokedToken string
	var connection *client
	e.mu.Lock()
	for token := range e.cfg.DeviceTokens {
		if deviceTokenID(token) == deviceID {
			revokedToken = token
			delete(e.cfg.DeviceTokens, token)
			connection = e.clients[token]
			delete(e.clients, token)
			break
		}
	}
	e.mu.Unlock()
	storeDeleted := e.devices.Delete(deviceID)
	if revokedToken == "" && !storeDeleted {
		return false
	}
	for _, s := range e.manager.List() {
		s.Unsubscribe(revokedToken)
	}
	if connection != nil && connection.conn != nil {
		_ = connection.conn.Close()
	}
	return true
}

func (e *Engine) deviceAuthorized(token string) bool {
	e.mu.RLock()
	_, ok := e.cfg.DeviceTokens[token]
	e.mu.RUnlock()
	if ok {
		return true
	}
	name, ok := e.devices.Lookup(token)
	if !ok {
		return false
	}
	e.mu.Lock()
	e.cfg.DeviceTokens[token] = name
	e.mu.Unlock()
	return true
}

func deviceTokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func constantTimeHeaderMatch(header string, want [sha256.Size]byte) bool {
	got := sha256.Sum256([]byte(header))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

func adminStatus(status StatusReport, claudeCommand, codexCommand string) adminproto.AgentStatus {
	sessions := make([]adminproto.SessionSnapshot, 0, len(status.Sessions))
	for _, s := range status.Sessions {
		sessions = append(sessions, adminproto.SessionSnapshot{
			SessionID: s.SessionID, Name: s.Name, Status: s.Status, Owner: s.Owner,
			Subscribers: s.Subscribers, CreatedAt: s.CreatedAt, Running: s.Running, Health: s.Health, IdleSeconds: s.IdleSeconds,
		})
	}
	return adminproto.AgentStatus{
		Addr: status.Addr, AgentVersion: status.AgentVersion, ClaudeVersion: status.ClaudeVersion,
		ClaudeBin: status.ClaudeBin, CodexVersion: status.CodexVersion, CodexBin: status.CodexBin,
		ClaudeCommand: claudeCommand, CodexCommand: codexCommand,
		DefaultWorkingDir: status.DefaultWorkingDir,
		DefaultPermission: status.DefaultPermission, MaxConcurrentSession: status.MaxConcurrentSession,
		ConnectedDevices: status.ConnectedDevices, Sessions: sessions,
	}
}

func writeAdminJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func adminTemplate(item protocol.TemplateInfo) adminproto.Template {
	return adminproto.Template{TemplateID: item.TemplateID, Label: item.Label, Prompt: item.Prompt}
}

func adminTemplates(items []protocol.TemplateInfo) []adminproto.Template {
	result := make([]adminproto.Template, 0, len(items))
	for _, item := range items {
		result = append(result, adminTemplate(item))
	}
	return result
}
