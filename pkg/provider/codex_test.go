package provider

import (
	"encoding/json"
	"testing"

	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

func TestCodexDescriptorDefinesProviderSpecificPermissions(t *testing.T) {
	descriptor := NewCodexAdapter("codex", nil, true, "").Descriptor()
	if descriptor.ID != CodexID || descriptor.Name != "Codex" || !descriptor.Available {
		t.Fatalf("descriptor=%+v", descriptor)
	}
	want := []string{"readOnly", "workspaceWrite", "fullAccess"}
	if len(descriptor.Permissions) != len(want) {
		t.Fatalf("permissions=%+v", descriptor.Permissions)
	}
	for i, option := range descriptor.Permissions {
		if option.ID != want[i] {
			t.Fatalf("permission[%d]=%q want %q", i, option.ID, want[i])
		}
	}
	if !descriptor.Permissions[2].Dangerous {
		t.Fatal("fullAccess must be dangerous")
	}
}

func TestCodexTranslatorMapsAgentCommandAndDone(t *testing.T) {
	translator := NewCodexAdapter("codex", nil, true, "")
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"item.completed","item":{"id":"a","type":"agent_message","text":"hello"}}`)), protocol.TypeToken, "hello", "", "")
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"item.started","item":{"id":"c","type":"command_execution","command":"git status"}}`)), protocol.TypeToolUse, "", "Bash", `{"command":"git status"}`)
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"turn.completed"}`)), protocol.TypeDone, "", "", "")
	if got := translator.TranslateOutput([]byte(`{"type":"item.completed","item":{"id":"c","type":"command_execution","command":"git status","status":"completed"}}`)); len(got) != 0 {
		t.Fatalf("completed command duplicated tool card: %q", got)
	}
}

func TestCodexTranslatorMapsFileChangesAndToolCalls(t *testing.T) {
	translator := NewCodexAdapter("codex", nil, true, "")
	got := translator.TranslateOutput([]byte(`{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"a.txt","kind":"add"},{"path":"b.txt","kind":"update"},{"path":"c.txt","kind":"delete"}]}}`))
	if len(got) != 3 {
		t.Fatalf("file changes=%q", got)
	}
	wantTools := []string{"Write", "Edit", "Delete"}
	for i, payload := range got {
		var message protocol.ToolUseMsg
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		if message.Tool != wantTools[i] || message.Input == "" {
			t.Fatalf("change[%d]=%+v", i, message)
		}
	}
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"item.started","item":{"type":"mcp_tool_call","server":"github","tool":"search","arguments":{"q":"repo"}}}`)), protocol.TypeToolUse, "", "MCP · github/search", `{"q":"repo"}`)
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"item.started","item":{"type":"web_search","query":"Codex docs"}}`)), protocol.TypeToolUse, "", "WebSearch", `{"query":"Codex docs"}`)
}

func TestCodexTranslatorMapsFailuresAndIgnoresInternalItems(t *testing.T) {
	translator := NewCodexAdapter("codex", nil, true, "")
	failure := translator.TranslateOutput([]byte(`{"type":"turn.failed","error":{"message":"request failed"}}`))
	if len(failure) != 2 {
		t.Fatalf("turn failure messages=%q", failure)
	}
	assertCodexMessage(t, failure[:1], protocol.TypeError, "request failed", "", "")
	assertCodexMessage(t, failure[1:], protocol.TypeDone, "", "", "")
	assertCodexMessage(t, translator.TranslateOutput([]byte(`{"type":"error","message":"reconnecting"}`)), protocol.TypeError, "reconnecting", "", "")
	for _, input := range []string{
		`{"type":"thread.started","thread_id":"thread"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"type":"error","message":"config warning"}}`,
		`{"type":"item.completed","item":{"type":"reasoning","text":"private"}}`,
		`{"type":"future.event"}`,
		`not-json`,
	} {
		if got := translator.TranslateOutput([]byte(input)); len(got) != 0 {
			t.Fatalf("internal event %q translated as %q", input, got)
		}
	}
}

func assertCodexMessage(t *testing.T, payloads [][]byte, wantType, wantContent, wantTool, wantInput string) {
	t.Helper()
	if len(payloads) != 1 {
		t.Fatalf("payloads=%q", payloads)
	}
	var got struct {
		Type, Content, Tool, Input, Message string
	}
	if err := json.Unmarshal(payloads[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != wantType || got.Tool != wantTool || got.Input != wantInput {
		t.Fatalf("got=%+v want type=%q tool=%q input=%q", got, wantType, wantTool, wantInput)
	}
	content := got.Content
	if got.Type == protocol.TypeError {
		content = got.Message
	}
	if content != wantContent {
		t.Fatalf("content=%q want %q", content, wantContent)
	}
}
