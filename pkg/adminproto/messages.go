// Package adminproto defines the loopback-only desktop administration protocol.
package adminproto

type Snapshot struct {
	Agent           AgentStatus      `json:"agent"`
	Devices         []DeviceSnapshot `json:"devices"`
	Projects        []Project        `json:"projects"`
	Diagnostics     Diagnostics      `json:"diagnostics"`
	PermissionRules []PermissionRule `json:"permissionRules"`
	Templates       []Template       `json:"templates"`
}

type Template struct {
	TemplateID string `json:"templateId"`
	Label      string `json:"label"`
	Prompt     string `json:"prompt"`
}

type UpdateSettingsRequest struct {
	DefaultWorkingDir     string `json:"defaultWorkingDir"`
	DefaultPermission     string `json:"defaultPermission"`
	MaxConcurrentSessions int    `json:"maxConcurrentSessions"`
	ClaudeCommand         string `json:"claudeCommand,omitempty"`
	CodexCommand          string `json:"codexCommand,omitempty"`
}

type PermissionRule struct {
	RuleID  string `json:"ruleId"`
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
}

type Diagnostics struct {
	GoVersion     string `json:"goVersion"`
	GOOS          string `json:"goos"`
	GOARCH        string `json:"goarch"`
	PID           int    `json:"pid"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
	Goroutines    int    `json:"goroutines"`
	AllocBytes    uint64 `json:"allocBytes"`
	SysBytes      uint64 `json:"sysBytes"`
	DataDir       string `json:"dataDir"`
	Caffeinating  bool   `json:"caffeinating"`
}

type Project struct {
	ProjectID  string `json:"projectId" yaml:"-"`
	Name       string `json:"name" yaml:"name"`
	Path       string `json:"path" yaml:"path"`
	Permission string `json:"permission" yaml:"permission,omitempty"`
}

type DeviceSnapshot struct {
	DeviceID string `json:"deviceId"`
	Name     string `json:"name"`
	Online   bool   `json:"online"`
}

type DeviceCredential struct {
	Device      DeviceSnapshot `json:"device"`
	DeviceToken string         `json:"deviceToken"`
}

type CreateDeviceRequest struct {
	Name string `json:"name"`
}

type AgentStatus struct {
	Addr                 string            `json:"addr"`
	AgentVersion         string            `json:"agentVersion"`
	ClaudeVersion        string            `json:"claudeVersion"`
	ClaudeBin            string            `json:"claudeBin"`
	CodexVersion         string            `json:"codexVersion"`
	CodexBin             string            `json:"codexBin"`
	ClaudeCommand        string            `json:"claudeCommand"`
	CodexCommand         string            `json:"codexCommand"`
	DefaultWorkingDir    string            `json:"defaultWorkingDir"`
	DefaultPermission    string            `json:"defaultPermission"`
	MaxConcurrentSession int               `json:"maxConcurrentSession"`
	ConnectedDevices     []string          `json:"connectedDevices"`
	Sessions             []SessionSnapshot `json:"sessions"`
}

type SessionSnapshot struct {
	SessionID   string   `json:"sessionId"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner"`
	Subscribers []string `json:"subscribers"`
	CreatedAt   int64    `json:"createdAt"`
	Running     bool     `json:"running"`
	Health      string   `json:"health"`
	IdleSeconds int64    `json:"idleSeconds"`
}

type StopSessionRequest struct {
	SessionID string `json:"sessionId"`
}
