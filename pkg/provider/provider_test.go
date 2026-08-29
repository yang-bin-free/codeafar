package provider

import "testing"

func TestNormalizeIDDefaultsLegacySessionsToClaude(t *testing.T) {
	if got := NormalizeID(""); got != ClaudeID {
		t.Fatalf("NormalizeID(\"\")=%q want %q", got, ClaudeID)
	}
}

func TestAdaptersPassBinArgsToSessionConfig(t *testing.T) {
	adapter := NewClaudeAdapter("/bin/wrapper", []string{"claude"})
	got := adapter.NewProcess(SessionConfig{Cwd: "/tmp", Permission: "default"})
	if got == nil {
		t.Fatal("nil process")
	}
}

func TestRegistryReturnsClaudeAndRejectsUnknownProvider(t *testing.T) {
	registry := NewRegistry(NewClaudeAdapter("claude", nil))
	adapter, ok := registry.Get("")
	if !ok || adapter.Descriptor().ID != ClaudeID {
		t.Fatalf("legacy provider lookup = %+v, %v", adapter, ok)
	}
	if _, ok := registry.Get("codex"); ok {
		t.Fatal("unregistered Codex provider was accepted")
	}
}

func TestClaudeDescriptorDefinesProviderSpecificPermissions(t *testing.T) {
	descriptor := NewClaudeAdapter("claude", nil).Descriptor()
	if descriptor.Name != "Claude Code" || len(descriptor.Permissions) != 4 {
		t.Fatalf("descriptor=%+v", descriptor)
	}
	want := []string{"default", "acceptEdits", "plan", "bypassPermissions"}
	for i, option := range descriptor.Permissions {
		if option.ID != want[i] {
			t.Fatalf("permission[%d]=%q want %q", i, option.ID, want[i])
		}
	}
	if !descriptor.Permissions[3].Dangerous {
		t.Fatal("bypassPermissions must be marked dangerous")
	}
}

func TestRegistryDescriptorsKeepRegistrationOrder(t *testing.T) {
	registry := NewRegistry(
		NewClaudeAdapter("claude", nil),
		NewCodexAdapter("codex", nil, true, ""),
	)
	for i := 0; i < 20; i++ {
		descriptors := registry.Descriptors()
		if len(descriptors) != 2 || descriptors[0].ID != ClaudeID || descriptors[1].ID != CodexID {
			t.Fatalf("descriptor order=%+v", descriptors)
		}
	}
}
