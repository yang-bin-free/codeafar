package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yang-bin-free/claude-phone/pkg/protocol"
)

// TestWrapperCommandEndToEnd proves that a configured multi-word launch
// command drives one full chat turn through the engine: the wrapper process
// must be spawned with the prepend argument first, receive the user message
// on stdin, and have its claude stream-json events streamed back to the
// WebSocket client as token/done protocol messages.
func TestWrapperCommandEndToEnd(t *testing.T) {
	// Fake wrapper CLI: it fails fast unless its first argument is the
	// configured prepend word, then behaves like `claude --print
	// --input-format stream-json --output-format stream-json` by answering
	// every stdin line with a deterministic claude stream-json turn.
	wrapperDir := t.TempDir()
	wrapper := filepath.Join(wrapperDir, "fake-wrapper")
	script := `#!/bin/sh
if [ "$1" != "claude" ]; then
  echo "wrapper got bad first arg: $1" >&2
  exit 1
fi
while read -r _line; do
  printf '%s\n' '{"type":"system","subtype":"init","session_id":"e2e-wrapper-session"}'
  printf '%s\n' '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi from wrapper"}}}'
  printf '%s\n' '{"type":"result","subtype":"success","result":"hi from wrapper","is_error":false}'
done
`
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	e := New(Config{
		DataDir:           t.TempDir(),
		ClaudeBin:         wrapper,
		ClaudeBinArgs:     []string{"claude"},
		DefaultWorkingDir: projectDir,
		DefaultPermission: "default",
		DeviceTokens:      map[string]string{"e2e-device": "E2E"},
	})
	defer e.Close()
	server, conn := openAuthenticatedEngine(t, e, "e2e-device")
	defer server.Close()
	defer conn.Close()

	writeJSON(t, conn, protocol.ControlMsg{
		Type: protocol.TypeControl, Action: protocol.ActionCreateSession,
		Name: "wrapper e2e", WorkingDir: projectDir, Provider: "claude",
		PermissionMode: "default", RequestID: "wrapper-e2e",
	})
	created := readSessionCreated(t, conn)
	if created.Provider != "claude" || created.Cwd != projectDir {
		t.Fatalf("created=%+v", created)
	}

	writeJSON(t, conn, protocol.TextMsg{Type: protocol.TypeText, Content: "run one turn"})

	// If the wrapper rejected its argv the turn produces an error (or nothing
	// at all before the deadline), so a completed turn carrying the wrapper's
	// canned token proves the prepend argument reached the spawned process.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var response strings.Builder
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatal(err)
		}
		switch envelope.Type {
		case protocol.TypeToken:
			var token protocol.TokenMsg
			if err := json.Unmarshal(payload, &token); err != nil {
				t.Fatal(err)
			}
			response.WriteString(token.Content)
		case protocol.TypeError:
			var message protocol.ErrorMsg
			_ = json.Unmarshal(payload, &message)
			t.Fatalf("wrapper turn failed: %+v", message)
		case protocol.TypeDone:
			if !strings.Contains(response.String(), "hi from wrapper") {
				t.Fatalf("assistant output %q does not contain the wrapper token", response.String())
			}
			return
		}
	}
}
