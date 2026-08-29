// Package provider defines the coding-agent boundary used by the engine.
package provider

import (
	"strings"

	"github.com/yang-bin-free/claude-phone/pkg/session"
)

const (
	ClaudeID = "claude"
	CodexID  = "codex"
)

type PermissionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Dangerous   bool   `json:"dangerous,omitempty"`
	Mutable     bool   `json:"mutable"`
}

type Descriptor struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Available         bool               `json:"available"`
	UnavailableReason string             `json:"unavailableReason,omitempty"`
	Permissions       []PermissionOption `json:"permissions"`
	Models            []string           `json:"models,omitempty"`
}

type SessionConfig struct {
	Cwd               string
	SessionID         string
	ProviderSessionID string
	Permission        string
	Model             string
	Resume            bool
	AddDirs           []string
	AllowedTools      []string
	BinArgs           []string
}

type Process interface {
	OnOutput(session.OutputFunc)
	Start() error
	Send(string) error
	Stop() error
}

// SessionIdentity is implemented by providers whose upstream conversation ID
// differs from CodeAfar's local session ID.
type SessionIdentity interface {
	ProviderSessionID() string
}

type Adapter interface {
	Descriptor() Descriptor
	NewProcess(SessionConfig) Process
}

// OutputTranslator converts a provider's native event stream into CodeAfar's
// shared WebSocket protocol. Providers without one use the legacy Claude
// translator in the engine.
type OutputTranslator interface {
	TranslateOutput(payload []byte) [][]byte
}

type Registry struct {
	byID  map[string]Adapter
	order []string
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{byID: make(map[string]Adapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		id := NormalizeID(adapter.Descriptor().ID)
		if _, exists := registry.byID[id]; !exists {
			registry.order = append(registry.order, id)
		}
		registry.byID[id] = adapter
	}
	return registry
}

func NormalizeID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return ClaudeID
	}
	return id
}

func (r *Registry) Get(id string) (Adapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.byID[NormalizeID(id)]
	return adapter, ok
}

func (r *Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, 0, len(r.byID))
	for _, id := range r.order {
		out = append(out, r.byID[id].Descriptor())
	}
	return out
}

func SupportsPermission(descriptor Descriptor, permission string) bool {
	for _, option := range descriptor.Permissions {
		if option.ID == permission {
			return true
		}
	}
	return false
}
