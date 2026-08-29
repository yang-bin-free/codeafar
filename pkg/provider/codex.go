package provider

import "github.com/yang-bin-free/claude-phone/pkg/session"

type CodexAdapter struct {
	bin               string
	binArgs           []string
	available         bool
	unavailableReason string
}

func NewCodexAdapter(bin string, binArgs []string, available bool, unavailableReason string) *CodexAdapter {
	if bin == "" {
		bin = "codex"
	}
	return &CodexAdapter{bin: bin, binArgs: binArgs, available: available, unavailableReason: unavailableReason}
}

func (a *CodexAdapter) Descriptor() Descriptor {
	return Descriptor{
		ID: CodexID, Name: "Codex", Available: a.available, UnavailableReason: a.unavailableReason,
		Permissions: []PermissionOption{
			{ID: "readOnly", Label: "只读", Description: "可检查和分析项目，但不能修改文件。", Mutable: true},
			{ID: "workspaceWrite", Label: "工作区访问", Description: "可在当前项目内修改文件并运行本地命令。", Mutable: true},
			{ID: "fullAccess", Label: "完全访问", Description: "取消文件系统和网络沙箱，仅用于完全信任的环境。", Dangerous: true, Mutable: true},
		},
	}
}

func (a *CodexAdapter) NewProcess(config SessionConfig) Process {
	return session.NewCodexProc(session.CodexConfig{
		Bin: a.bin, BinArgs: a.binArgs, Cwd: config.Cwd, ProviderSessionID: config.ProviderSessionID,
		Permission: config.Permission, Model: config.Model, AddDirs: config.AddDirs,
	})
}
