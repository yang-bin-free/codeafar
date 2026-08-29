package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestDesktopAdminFormsHavePersistentLabelsAndMode(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	for _, id := range []string{
		"device-name", "project-name", "project-path", "template-label",
		"template-prompt", "permission-tool", "permission-pattern",
	} {
		if !strings.Contains(html, `for="`+id+`"`) {
			t.Errorf("admin control %s has no persistent label", id)
		}
	}
	for _, id := range []string{"settings-claude-command", "settings-codex-command"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("settings form missing CLI command input %s", id)
		}
	}

	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	if !strings.Contains(js, `classList.toggle("admin-mode"`) {
		t.Error("chat shell does not expose admin mode to presentation")
	}

	adminBytes, err := fs.ReadFile(Assets, "admin/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	admin := string(adminBytes)
	if !strings.Contains(admin, `settings-claude-command`) || !strings.Contains(admin, `settings-codex-command`) {
		t.Error("admin JS does not handle CLI command settings")
	}
}

func TestDesktopDiagnosticsUseCompactSummary(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlBytes), `class="metric-summary"`) {
		t.Error("diagnostics should use one compact summary region")
	}

	jsBytes, err := fs.ReadFile(Assets, "admin/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsBytes), `class="metric-item"`) {
		t.Error("diagnostics items should use the flat summary presentation")
	}
}

func TestDesktopThemeUsesWarmAccentAndMacSizedControls(t *testing.T) {
	cssBytes, err := fs.ReadFile(Assets, "chat/core.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, marker := range []string{"--accent: #d97757", "min-height: 44px", "var(--accent)"} {
		if !strings.Contains(css, marker) {
			t.Errorf("desktop theme missing %q", marker)
		}
	}
	if strings.Contains(css, "#7c5cff") {
		t.Error("legacy generic-purple accent is still active")
	}
}

func TestDesktopDestructiveActionsRequireConfirmation(t *testing.T) {
	adminBytes, err := fs.ReadFile(Assets, "admin/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	admin := string(adminBytes)
	for _, marker := range []string{
		"confirmDangerousAction", "确认吊销这个设备", "确认删除这个工作目录",
		"确认删除这个提示词模板", "确认删除这条权限规则", "确认停止这个会话",
	} {
		if !strings.Contains(admin, marker) {
			t.Errorf("admin destructive flow missing %q", marker)
		}
	}

	chatBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chatBytes), "确认停止当前会话") {
		t.Error("chat session stop does not ask for confirmation")
	}
}

func TestDesktopEmptyStateExplainsTheNextAction(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	for _, marker := range []string{"选择工作目录", "⌘N", `class="empty-title"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("desktop empty state missing %q", marker)
		}
	}
}

func TestDesktopComposerWaitsForSessionSelectionAfterReconnect(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		"sessionReady: false", "function updateControls()", "state.sessionReady = false",
		"state.sessionReady = true", `action: "select_session", sessionId: state.sessionId`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("chat reconnect guard missing %q", marker)
		}
	}
}

func TestDesktopMessagesAreSelectable(t *testing.T) {
	cssBytes, err := fs.ReadFile(Assets, "chat/desktop.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, marker := range []string{`.message {`, `-webkit-user-select: text`, `user-select: text`, `cursor: text`} {
		if !strings.Contains(css, marker) {
			t.Errorf("message selection rule missing %q", marker)
		}
	}
}

func TestDesktopMessagesExposeCopyAction(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`button.className = "message-copy"`,
		`button.setAttribute("aria-label", "复制消息")`,
		`await writeClipboard(content.textContent)`,
		`window.codeAfarNative?.copyText`,
		`document.execCommand("copy")`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("message copy action missing %q", marker)
		}
	}
}

func TestDesktopReturnSendsWithoutBreakingIME(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	marker := `prompt.addEventListener("keydown", event => {
    if (event.isComposing || event.keyCode === 229) return;
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      composer.requestSubmit();
    }
  });`
	if !strings.Contains(js, marker) {
		t.Fatal("composition-safe Return-to-send handler missing")
	}
}

func TestNewSessionUsesComposerContextBar(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(htmlBytes)
	headerEnd := strings.Index(html, "</header>")
	if headerEnd < 0 {
		t.Fatal("desktop header is missing")
	}
	header := html[:headerEnd]
	for _, removed := range []string{`id="create-project"`, `id="create-permission"`, "默认目录", ">严格<"} {
		if strings.Contains(header, removed) {
			t.Errorf("legacy header control remains: %q", removed)
		}
	}
	for _, marker := range []string{
		`id="draft-project"`, `id="draft-permission"`,
		`class="composer-context"`, "告诉 CodeAfar 要做什么",
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("draft composer missing %q", marker)
		}
	}
}

func TestProviderSwitcherOwnsNewSessionAndHistory(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	cssBytes, err := fs.ReadFile(Assets, "chat/desktop.css")
	if err != nil {
		t.Fatal(err)
	}
	html, js, css := string(htmlBytes), string(jsBytes), string(cssBytes)
	combined := html + js + css
	for _, marker := range []string{
		`class="provider-toolbar"`, `id="provider-switcher"`, `id="provider-switcher-mobile"`,
		`aria-label="在当前引擎中新建会话"`, `function switchProvider(providerID)`,
		`providerWorkspace.sessionsForProvider(state.sessions, state.activeProvider)`,
		`provider: state.activeProvider`, `lastSessions`,
	} {
		if !strings.Contains(combined, marker) {
			t.Errorf("provider workspace missing %q", marker)
		}
	}
	for _, forbidden := range []string{`id="draft-provider"`, `id="provider-label"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("duplicate composer provider control remains %q", forbidden)
		}
	}
}

func TestChatScriptCreatesThenSendsPendingFirstPrompt(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`function beginDraft(confirmDiscard = true)`, `status: "draft"`, `requestId: newRequestID()`,
		`firstPrompt: content`, `action: "create_session"`, `requestId: state.draft.requestId`,
		`case "session_created":`, `pendingFirstPrompt`, `case "text_accepted":`,
		`requestId: state.pendingFirstPrompt.requestId`, `deliverPendingFirstPrompt()`,
		`state.supportsTextAck = Number.parseInt(msg.protocolVersion || "1", 10) >= 2`,
		`if (delivered && !state.supportsTextAck)`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("draft state machine missing %q", marker)
		}
	}
}

func TestChatScriptExposesCodeAfarBridgeWithLegacyAlias(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`window.codeAfar = bridge`, `window.claudePhone = window.codeAfar`,
		`async function chooseProjectDirectory()`, `window.codeAfarNative?.chooseDirectory`,
		`fetch("/desktop/projects"`, `"X-CodeAfar-Admin-Token": token`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("CodeAfar bridge/folder picker missing %q", marker)
		}
	}
}

func TestPermissionSelectorTracksOnlyConfirmedMode(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`permissionMode: "default"`, `draft.permissionMode = requested`,
		`permissionSelect.value = confirmed`, `state.pendingPermission = { sessionId, requested, confirmed }`,
		`state.pendingPermission?.sessionId === state.sessionId`, `permissionMode: state.pendingPermission.requested`,
		`const draft = state.draft`, `const sessionId = state.sessionId`,
		`const contextMatches = draft ? state.draft === draft`, `state.sessionId === sessionId`,
		`state.pendingPermission = null`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("permission rollback missing %q", marker)
		}
	}
}

func TestDesktopChatPinsComposerAndKeepsMessagesSelectable(t *testing.T) {
	cssBytes, err := fs.ReadFile(Assets, "chat/desktop.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, marker := range []string{
		`body.desktop { height: 100vh; overflow: hidden; }`,
		`.desktop .app-shell { height: 100vh; min-height: 0; overflow: hidden; }`,
		`.desktop .workspace { height: 100vh; min-height: 0; overflow: hidden; }`,
		`.desktop .chat-view { min-height: 0; overflow: hidden; }`,
		`.desktop .messages { min-height: 0; overflow-y: auto; }`,
		`-webkit-user-select: text`, `opacity: .55`,
	} {
		if !strings.Contains(css, marker) {
			t.Errorf("desktop chat layout/copy affordance missing %q", marker)
		}
	}
}

func TestDangerousActionsUseInAppConfirmation(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	adminBytes, err := fs.ReadFile(Assets, "admin/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	html, js, admin := string(htmlBytes), string(jsBytes), string(adminBytes)
	for _, marker := range []string{`id="confirm-dialog"`, `id="confirm-message"`, `value="confirm"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("confirmation dialog missing %q", marker)
		}
	}
	if !strings.Contains(js, `function requestConfirmation(message)`) || !strings.Contains(js, `await requestConfirmation(`) {
		t.Error("dangerous actions do not use the in-app confirmation dialog")
	}
	if strings.Contains(js, "window.confirm(") {
		t.Error("WKWebView-incompatible window.confirm remains")
	}
	if strings.Contains(admin, "window.confirm(") || !strings.Contains(admin, `window.claudePhone.requestConfirmation(message)`) {
		t.Error("admin dangerous actions do not use the shared in-app confirmation dialog")
	}
	for _, marker := range []string{`confirmCancel.focus()`, `typeof confirmDialog.showModal`, `confirmDialog.classList.add("fallback")`} {
		if !strings.Contains(js, marker) {
			t.Errorf("confirmation compatibility behavior missing %q", marker)
		}
	}
	cssBytes, err := fs.ReadFile(Assets, "chat/core.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(cssBytes)
	for _, marker := range []string{`#confirm-dialog:not([open]) { display: none; }`, `width: 100vw; height: 100vh`, `#confirm-dialog.fallback .confirm-card`} {
		if !strings.Contains(css, marker) {
			t.Errorf("confirmation fallback missing %q", marker)
		}
	}
	for _, marker := range []string{`event.key === "Escape"`, `event.key !== "Tab"`, `event.target === confirmDialog`} {
		if !strings.Contains(js, marker) {
			t.Errorf("confirmation keyboard/modal behavior missing %q", marker)
		}
	}
	if !strings.Contains(js, `if (cancelActiveConfirmation !== null)`) || !strings.Contains(js, `if (event.metaKey) event.preventDefault()`) {
		t.Error("global shortcuts are not blocked while confirmation is active")
	}
}

func TestVoiceDraftAppendsWithoutSubmitting(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`voiceBase: ""`, `state.voiceBase = prompt.value`,
		`const separator = state.voiceBase && value`,
		"prompt.value = `${state.voiceBase}${separator}${value || \"\"}`",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("voice draft behavior missing %q", marker)
		}
	}
	start := strings.Index(js, "setVoiceText(value")
	end := strings.Index(js[start:], "setVoiceState(")
	if start < 0 || end < 0 {
		t.Fatal("voice bridge methods are missing")
	}
	if strings.Contains(js[start:start+end], "requestSubmit") || strings.Contains(js[start:start+end], `send({ type: "text"`) {
		t.Fatal("voice transcript must not submit itself")
	}
}

func TestDirectoryPickerCanBeRetriedAfterCancelOrFailure(t *testing.T) {
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsBytes)
	for _, marker := range []string{
		`finally {`,
		`if (projectSelect.value === "__choose__") projectSelect.value = ""`,
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("directory retry behavior missing %q", marker)
		}
	}
}

func TestInternalToolActivityDoesNotEnterChat(t *testing.T) {
	htmlBytes, err := fs.ReadFile(Assets, "chat/index.html")
	if err != nil {
		t.Fatal(err)
	}
	jsBytes, err := fs.ReadFile(Assets, "chat/chat.js")
	if err != nil {
		t.Fatal(err)
	}
	cssBytes, err := fs.ReadFile(Assets, "chat/desktop.css")
	if err != nil {
		t.Fatal(err)
	}
	combined := string(htmlBytes) + string(jsBytes) + string(cssBytes)
	for _, forbidden := range []string{
		"tool-format.js", "formatToolUse", `append("tool"`,
		`Bash: ["执行命令"`, `.message.tool`,
	} {
		if strings.Contains(combined, forbidden) {
			t.Errorf("internal tool UI remains %q", forbidden)
		}
	}
	if !strings.Contains(string(jsBytes), "case \"tool_use\":\n        break;") {
		t.Error("live tool events must be explicitly ignored")
	}
}
