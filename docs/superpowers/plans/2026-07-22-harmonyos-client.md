# CodeAfar HarmonyOS NEXT V1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a native HarmonyOS NEXT CodeAfar client and a Mac-only Tailscale Funnel gateway that provide secure pairing, Claude/Codex workspaces, chat, permissions, reconnect, copy, and strictly on-device-gated speech without operating a CodeAfar cloud service.

**Architecture:** The Harmony app is a native ArkTS/ArkUI Stage application that consumes the existing JSON WebSocket protocol. The Mac app adds a second loopback-only remote gateway exposing only `/pair`, `/ws`, and `/healthz`; Tailscale Funnel forwards public TLS/WSS traffic to that gateway while the existing desktop/admin listener remains private. Pairing uses short-lived single-use codes to mint the existing per-device credentials.

**Tech Stack:** Go 1.26, `net/http`, Gorilla WebSocket, Tailscale CLI 1.98+, ArkTS, ArkUI, Stage model, HarmonyOS Network Kit, Asset Store Kit, Test Kit, DevEco Studio, hvigor, XCTest/JUnit-style contract tests already present in the repository.

## Global Constraints

- Target HarmonyOS NEXT with ArkTS, ArkUI, and the Stage model; do not ship an Android compatibility package.
- Do not add a CodeAfar relay, cloud account, cloud history, analytics, or push service.
- Do not port Tailscale, WireGuard, a TUN device, subnet routing, or a general VPN into the Harmony app.
- Funnel must forward to a dedicated loopback listener and must never expose the desktop UI, `/admin`, `/desktop/projects`, diagnostics, or static assets.
- Use Tailscale Funnel port `8443`; detect an existing user-owned configuration and never overwrite it.
- Pairing codes are single-use, expire after five minutes, are attempt-limited, and are never placed in an HTTP query string.
- Device tokens remain per-device and revocable; store them through HarmonyOS secure asset storage, not preferences or logs.
- Created sessions keep their provider permanently; provider histories and last-selected sessions remain isolated.
- `tool_use` protocol events never create ordinary chat messages.
- Speech only fills an editable draft, never auto-sends, and is enabled only when an official runtime API explicitly guarantees on-device/offline recognition for the current device and language.
- Do not ignore TLS errors or fall back from WSS to plaintext WebSocket.
- A source tree without a passing HAP build and simulator acceptance is not complete.

---

## File Map

### Mac and shared Go

- `pkg/engine/devices.go`: exported device creation/revocation service used by admin and pairing.
- `pkg/remote/pairing.go`: in-memory one-time pairing manager with expiry and attempt limits.
- `pkg/remote/gateway.go`: allowlisted remote HTTP/WebSocket surface and request limits.
- `pkg/remote/funnel.go`: Tailscale CLI detection, conflict-safe enable/status/disable logic.
- `pkg/remote/*_test.go`: deterministic pairing, route-isolation, and Funnel command tests.
- `cmd/mac-app/application.go`: owns the second loopback listener and remote lifecycle.
- `cmd/mac-app/main.go`: adds the fixed-default remote listener flag and wires the controller.
- `pkg/desktop/server.go`: authenticated local-only remote settings API for the Mac UI.
- `web/admin/admin.js`, `web/chat/index.html`, `web/assets/admin.css`: remote access controls and QR presentation.
- `pkg/product/harmony_contract_test.go`: cross-platform policy and packaging contract.

### HarmonyOS NEXT

- `harmony/AppScope/app.json5`: application identity and version.
- `harmony/build-profile.json5`, `harmony/hvigorfile.ts`, `harmony/oh-package.json5`: root DevEco/hvigor configuration.
- `harmony/entry/src/main/module.json5`: Stage module, network and microphone permissions.
- `harmony/entry/src/main/ets/protocol/Models.ets`: wire types.
- `harmony/entry/src/main/ets/protocol/Codec.ets`: strict inbound decode and outbound encode.
- `harmony/entry/src/main/ets/networking/PairingClient.ets`: QR payload validation and `/pair` exchange.
- `harmony/entry/src/main/ets/networking/CodeAfarSocket.ets`: authenticated WSS, reconnect, lifecycle.
- `harmony/entry/src/main/ets/security/CredentialStore.ets`: Asset Store-backed credential storage.
- `harmony/entry/src/main/ets/stores/ConnectionStore.ets`: connection state and user-facing errors.
- `harmony/entry/src/main/ets/stores/SessionStore.ets`: providers, projects, sessions, per-provider restoration.
- `harmony/entry/src/main/ets/stores/ChatStore.ets`: history, streaming, first-message transaction, drafts.
- `harmony/entry/src/main/ets/speech/SpeechController.ets`: on-device policy gate and draft callbacks.
- `harmony/entry/src/main/ets/pages/*.ets`: pairing, session, chat, and settings pages.
- `harmony/entry/src/main/ets/components/*.ets`: provider row, session list, message bubble, composer, and new-session sheet.
- `harmony/entry/src/ohosTest/ets/test/*.test.ets`: protocol, store, network, speech, and UI tests.
- `scripts/validate-harmony-project.sh`, `scripts/test-harmony.sh`: reproducible project checks and build/test entrypoints.

---

### Task 1: DevEco Toolchain and Native Project Contract

**Files:**
- Create: `pkg/product/harmony_contract_test.go`
- Create: `harmony/AppScope/app.json5`
- Create: `harmony/build-profile.json5`
- Create: `harmony/hvigorfile.ts`
- Create: `harmony/hvigorw`
- Create: `harmony/oh-package.json5`
- Create: `harmony/hvigor/hvigor-config.json5`
- Create: `harmony/entry/build-profile.json5`
- Create: `harmony/entry/hvigorfile.ts`
- Create: `harmony/entry/oh-package.json5`
- Create: `harmony/entry/src/main/module.json5`
- Create: `harmony/entry/src/main/ets/entryability/EntryAbility.ets`
- Create: `harmony/entry/src/main/ets/pages/Index.ets`
- Create: `scripts/validate-harmony-project.sh`
- Create: `scripts/test-harmony.sh`
- Modify: `.gitignore`
- Modify: `Makefile`

**Interfaces:**
- Produces: a buildable `com.codeafar.harmony` Stage application and the stable commands `make harmony-validate`, `make harmony-test`, and `make harmony-hap`.
- Consumes: no feature code.

- [ ] **Step 1: Write the failing repository contract**

```go
func TestHarmonyClientProjectContract(t *testing.T) {
    repo := repoRoot(t)
    required := []string{
        "harmony/AppScope/app.json5",
        "harmony/entry/src/main/module.json5",
        "harmony/entry/src/main/ets/pages/Index.ets",
        "scripts/validate-harmony-project.sh",
        "scripts/test-harmony.sh",
    }
    for _, name := range required {
        if _, err := os.Stat(filepath.Join(repo, name)); err != nil {
            t.Errorf("missing HarmonyOS project file %s: %v", name, err)
        }
    }
}
```

- [ ] **Step 2: Run the contract and record the expected failure**

Run: `go test ./pkg/product -run TestHarmonyClientProjectContract -count=1`  
Expected: FAIL listing the missing `harmony/` project files.

- [ ] **Step 3: Install and verify the official toolchain**

Install the current stable Apple-silicon DevEco Studio from Huawei Developer into `/Applications/DevEco-Studio.app`, launch it once, accept the SDK license, and install the HarmonyOS NEXT SDK with API 12 or newer plus a phone emulator image.

Run:

```bash
test -d /Applications/DevEco-Studio.app
find "$HOME/Library/Huawei" "$HOME/.huawei" -maxdepth 6 -type f -name hvigorw 2>/dev/null | head -1
```

Expected: DevEco exists and an SDK/hvigor installation is discoverable. Record the actual SDK root in the local environment only; do not commit a user-specific absolute path.

- [ ] **Step 4: Generate the minimal Stage project and validation scripts**

Use DevEco's Empty Ability template with bundle name `com.codeafar.harmony`, module `entry`, ArkTS, Stage model, and minimum compatible API 12. Keep generated dependency versions intact.

`scripts/validate-harmony-project.sh` must perform these exact policy checks:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
required=(
  harmony/AppScope/app.json5
  harmony/build-profile.json5
  harmony/entry/src/main/module.json5
  harmony/entry/src/main/ets/entryability/EntryAbility.ets
  harmony/entry/src/main/ets/pages/Index.ets
)
for file in "${required[@]}"; do test -f "$file"; done
rg -q 'com\.codeafar\.harmony' harmony/AppScope/app.json5 harmony/entry/src/main/module.json5
! rg -n 'dt_[A-Za-z0-9_-]{16,}|tskey-auth-|adminToken|deviceToken\s*:' harmony --glob '!**/*test*'
echo "HarmonyOS project structure OK"
```

`scripts/test-harmony.sh` must resolve DevEco's bundled Node/ohpm environment and run:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../harmony"
./hvigorw test --mode module -p module=entry@default --no-daemon
./hvigorw clean --mode module -p product=default assembleHap --no-daemon
```

- [ ] **Step 5: Wire Make targets and verify the green state**

```make
harmony-validate:
	./scripts/validate-harmony-project.sh

harmony-test:
	./scripts/test-harmony.sh

harmony-hap:
	cd harmony && ./hvigorw clean --mode module -p product=default assembleHap --no-daemon
```

Run:

```bash
go test ./pkg/product -run TestHarmonyClientProjectContract -count=1
make harmony-validate
make harmony-hap
```

Expected: PASS, `HarmonyOS project structure OK`, and a HAP under `harmony/entry/build/default/outputs/default/`.

- [ ] **Step 6: Commit the project foundation**

```bash
git add .gitignore Makefile harmony scripts/validate-harmony-project.sh scripts/test-harmony.sh pkg/product/harmony_contract_test.go
git commit -m "build: scaffold HarmonyOS client"
```

---

### Task 2: Engine Credential Service and Single-Use Pairing

**Files:**
- Modify: `pkg/engine/devices.go`
- Modify: `pkg/engine/admin.go`
- Modify: `cmd/mac-app/application.go`
- Create: `pkg/remote/pairing.go`
- Create: `pkg/remote/pairing_test.go`

**Interfaces:**
- Produces: `engine.DeviceCredentials` with `CreateDevice(name string) (adminproto.DeviceCredential, error)` and `RevokeDevice(deviceID string) bool`.
- Produces: `remote.PairingManager.Issue(endpoint string) (PairingOffer, error)` and `Redeem(code, deviceName string) (adminproto.DeviceCredential, error)`.
- Consumes: the existing persistent `deviceStore` and device revocation behavior.

- [ ] **Step 1: Write failing expiry, replay, attempt, and persistence tests**

```go
func TestPairingCodeIsSingleUseAndExpires(t *testing.T) {
    now := time.Unix(1000, 0)
    issuer := &fakeIssuer{}
    manager := NewPairingManager(issuer, func() time.Time { return now })
    offer, err := manager.Issue("wss://mac.example.ts.net:8443/ws")
    if err != nil { t.Fatal(err) }
    credential, err := manager.Redeem(offer.Code, "Harmony Phone")
    if err != nil || credential.DeviceToken == "" { t.Fatalf("redeem: %+v %v", credential, err) }
    if _, err := manager.Redeem(offer.Code, "Replay"); !errors.Is(err, ErrPairingInvalid) {
        t.Fatalf("replay err=%v", err)
    }
    expired, _ := manager.Issue("wss://mac.example.ts.net:8443/ws")
    now = now.Add(5*time.Minute + time.Second)
    if _, err := manager.Redeem(expired.Code, "Late"); !errors.Is(err, ErrPairingExpired) {
        t.Fatalf("expiry err=%v", err)
    }
}
```

Add a second test that makes five invalid attempts and expects `ErrPairingAttemptsExceeded`, and an engine test that creates a device, restarts the Engine, authenticates it, then revokes it.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./pkg/remote ./pkg/engine -run 'TestPairing|TestEngineDeviceCredentials' -count=1`  
Expected: FAIL because `pkg/remote` and the exported credential service do not exist.

- [ ] **Step 3: Export one credential implementation from Engine**

Add these methods and make the admin endpoints call them instead of duplicating store/map mutation:

```go
func (e *Engine) CreateDevice(name string) (adminproto.DeviceCredential, error) {
    credential, err := e.devices.Add(name)
    if err != nil { return adminproto.DeviceCredential{}, err }
    e.mu.Lock()
    e.cfg.DeviceTokens[credential.DeviceToken] = credential.Device.Name
    e.mu.Unlock()
    return credential, nil
}

func (e *Engine) RevokeDevice(deviceID string) bool {
    return e.revokeDevice(deviceID)
}
```

Extend `managedEngine` with the same two methods.

- [ ] **Step 4: Implement the in-memory pairing manager**

Define:

```go
const pairingTTL = 5 * time.Minute
const pairingMaxAttempts = 5

type CredentialIssuer interface {
    CreateDevice(string) (adminproto.DeviceCredential, error)
}

type PairingOffer struct {
    Endpoint  string    `json:"endpoint"`
    Code      string    `json:"code"`
    ExpiresAt time.Time `json:"expiresAt"`
}

type pairingEntry struct {
    hash [sha256.Size]byte
    expiresAt time.Time
    attempts int
}
```

Generate 32 random bytes, expose the code with base64 raw-URL encoding, store only SHA-256, compare with `subtle.ConstantTimeCompare`, delete on success, and delete expired/exhausted entries. `Redeem` calls `CreateDevice` only after validation.

- [ ] **Step 5: Run focused and engine tests**

Run:

```bash
go test ./pkg/remote ./pkg/engine -run 'TestPairing|TestEngineDeviceCredentials|TestAdmin' -count=1
go test -race ./pkg/remote ./pkg/engine -count=1
```

Expected: PASS with no data race.

- [ ] **Step 6: Commit credential and pairing behavior**

```bash
git add pkg/engine/devices.go pkg/engine/admin.go cmd/mac-app/application.go pkg/remote
git commit -m "feat: add single-use remote pairing"
```

---

### Task 3: Allowlisted Remote Gateway

**Files:**
- Create: `pkg/remote/gateway.go`
- Create: `pkg/remote/gateway_test.go`
- Modify: `pkg/product/harmony_contract_test.go`

**Interfaces:**
- Produces: `remote.NewGateway(GatewayOptions) http.Handler`.
- Consumes: `PairingManager.Redeem`, the existing Engine handler at `/ws`, and a source-IP limiter.

- [ ] **Step 1: Write failing route-isolation and request-limit tests**

```go
func TestGatewayExposesOnlyPairWSAndHealth(t *testing.T) {
    gateway := NewGateway(GatewayOptions{EngineHandler: markerHandler(), Pairing: fakePairing()})
    cases := []struct{ method, path string; want int }{
        {"GET", "/healthz", 200}, {"POST", "/pair", 201}, {"GET", "/ws", 299},
        {"GET", "/", 404}, {"GET", "/admin/status", 404},
        {"POST", "/desktop/projects", 404}, {"GET", "/assets/chat.js", 404},
    }
    for _, tc := range cases {
        req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(validPairBody(tc.path)))
        req.RemoteAddr = "203.0.113.10:4000"
        rec := httptest.NewRecorder()
        gateway.ServeHTTP(rec, req)
        if rec.Code != tc.want { t.Errorf("%s %s=%d", tc.method, tc.path, rec.Code) }
    }
}
```

Add tests for a non-JSON pair request, a body above 4 KiB, six invalid attempts from one address, missing Engine handler, and `/healthz` containing exactly `ok\n`.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./pkg/remote -run TestGateway -count=1`  
Expected: FAIL because `NewGateway` is undefined.

- [ ] **Step 3: Implement the exact gateway surface**

```go
type GatewayOptions struct {
    EngineHandler func() http.Handler
    Pairing        interface {
        Redeem(code, deviceName string) (adminproto.DeviceCredential, error)
    }
    Limiter *IPLimiter
}

func NewGateway(options GatewayOptions) http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /healthz", health)
    mux.HandleFunc("POST /pair", pairHandler(options))
    mux.Handle("GET /ws", availableEngineWS(options.EngineHandler))
    return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mux.ServeHTTP(w, r)
    }))
}
```

Use `http.MaxBytesReader(..., 4096)`, require `application/json`, return only the credential response on success, map invalid/expired/exhausted codes to generic `PAIRING_REJECTED`, and add `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, and a strict no-referrer policy.

- [ ] **Step 4: Bound WebSocket frames in Engine**

Immediately after upgrade, call `conn.SetReadLimit(1 << 20)` and set an authentication read deadline of 10 seconds; clear the deadline after successful auth. Add focused WebSocket tests for oversized and non-auth first frames.

- [ ] **Step 5: Run security-focused tests**

Run:

```bash
go test ./pkg/remote ./pkg/engine -run 'TestGateway|TestWebSocket' -count=1
go test -race ./pkg/remote ./pkg/engine -count=1
```

Expected: PASS; forbidden paths remain 404 and never reach the Engine/admin handlers.

- [ ] **Step 6: Commit the gateway**

```bash
git add pkg/remote pkg/engine/wsserver.go pkg/engine/wsserver_test.go pkg/product/harmony_contract_test.go
git commit -m "feat: add isolated remote gateway"
```

---

### Task 4: Conflict-Safe Tailscale Funnel Controller

**Files:**
- Create: `pkg/remote/funnel.go`
- Create: `pkg/remote/funnel_test.go`
- Create: `pkg/remote/testdata/funnel-empty.json`
- Create: `pkg/remote/testdata/funnel-owned.json`
- Create: `pkg/remote/testdata/funnel-conflict.json`

**Interfaces:**
- Produces: `FunnelController.Status(context.Context) (FunnelStatus, error)`, `Enable(context.Context, string) (FunnelStatus, error)`, and `Disable(context.Context) error`.
- Consumes: the `tailscale` CLI through injectable `CommandRunner.Run(ctx, args...)`.

- [ ] **Step 1: Capture the installed CLI schema and write failing fixture tests**

Run read-only commands:

```bash
tailscale version
tailscale funnel status --json
tailscale funnel --help
```

Sanitize host/user identifiers before saving fixture shapes. Write tests that expect:

```go
func TestFunnelEnableDoesNotOverwriteConflict(t *testing.T) {
    runner := newFixtureRunner("funnel-conflict.json")
    controller := NewFunnelController(runner, StatePath(t.TempDir()))
    _, err := controller.Enable(context.Background(), "http://127.0.0.1:9878")
    if !errors.Is(err, ErrFunnelPortInUse) { t.Fatalf("err=%v", err) }
    if runner.MutatingCalls() != 0 { t.Fatalf("mutated conflicting config") }
}
```

Add tests that enable an empty port with `funnel --bg --https=8443`, recognize an owned configuration after restart, and refuse to disable when the live target no longer matches the stored fingerprint.

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./pkg/remote -run TestFunnel -count=1`  
Expected: FAIL because the Funnel controller does not exist.

- [ ] **Step 3: Implement command execution and state**

Define:

```go
type CommandRunner interface {
    Run(context.Context, ...string) ([]byte, error)
}

type FunnelStatus struct {
    Available bool   `json:"available"`
    Enabled   bool   `json:"enabled"`
    Endpoint  string `json:"endpoint,omitempty"`
    Reason    string `json:"reason,omitempty"`
}

type funnelOwnership struct {
    Port      int    `json:"port"`
    LocalURL  string `json:"localUrl"`
    Endpoint  string `json:"endpoint"`
}
```

Persist ownership atomically with mode `0600` under `~/.codeafar/funnel.json`. Status parsing must use the observed 1.98 JSON fields and reject unknown/conflicting shapes instead of guessing. User-visible reasons must be allowlisted (`Tailscale 未安装`, `Tailscale 未登录`, `8443 端口已被占用`, `Funnel 未启用`).

- [ ] **Step 4: Implement safe enable and disable**

Enable sequence: version/status checks, conflict check, run `funnel --bg --https=8443 <localURL>`, re-read status, verify exact target, then persist ownership. Disable sequence: re-read status, compare live port/target/endpoint to ownership, run `funnel --https=8443 off`, verify removal, then delete ownership.

- [ ] **Step 5: Run fixture, race, and installed-CLI read-only tests**

```bash
go test ./pkg/remote -run TestFunnel -count=1
go test -race ./pkg/remote -count=1
tailscale status --json >/dev/null
```

Expected: PASS. Do not enable the real Funnel until Task 11 acceptance.

- [ ] **Step 6: Commit Funnel management**

```bash
git add pkg/remote/funnel.go pkg/remote/funnel_test.go pkg/remote/testdata
git commit -m "feat: manage CodeAfar Funnel safely"
```

---

### Task 5: Mac Remote Lifecycle and Admin UI

**Files:**
- Modify: `cmd/mac-app/application.go`
- Modify: `cmd/mac-app/application_test.go`
- Modify: `cmd/mac-app/main.go`
- Modify: `pkg/desktop/server.go`
- Modify: `pkg/desktop/server_test.go`
- Modify: `web/chat/index.html`
- Modify: `web/admin/admin.js`
- Modify: `web/assets/admin.css`
- Modify: `web/design_regression_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Produces: a second listener at `127.0.0.1:9878`, local admin endpoints `GET/POST/DELETE /desktop/remote`, and `POST /desktop/remote/pairing` returning `{ endpoint, code, expiresAt, pairingURI, qrPNGBase64 }`.
- Consumes: `remote.NewGateway`, `PairingManager`, `FunnelController`, and Engine credentials.

- [ ] **Step 1: Write failing application and handler tests**

Test that `application.Start()` owns two loopback listeners, closing the application closes both, pausing the Engine makes remote `/ws` unavailable, and remote gateway paths cannot access admin. Add desktop handler tests that all remote-management endpoints reject missing/wrong admin tokens and non-loopback callers.

```go
func TestRemoteManagementRequiresLocalAdminToken(t *testing.T) {
    handler := NewHandler(HandlerOptions{AdminToken: "secret", Remote: fakeRemoteController{}})
    request := httptest.NewRequest(http.MethodPost, "/desktop/remote", nil)
    request.RemoteAddr = "203.0.113.9:5000"
    request.Header.Set("X-CodeAfar-Admin-Token", "secret")
    response := httptest.NewRecorder()
    handler.ServeHTTP(response, request)
    if response.Code != http.StatusForbidden { t.Fatalf("status=%d", response.Code) }
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./cmd/mac-app ./pkg/desktop -run 'TestRemote|TestApplication' -count=1`  
Expected: FAIL for missing remote controller/listener fields.

- [ ] **Step 3: Wire the second listener and remote controller**

Add `RemoteAddr string` to `appConfig` with CLI default `127.0.0.1:9878`. Reuse `validateDesktopAddr` for both listeners. Add `remoteListener`, `remoteDone`, `pairing`, and `funnel` to `application`; start `remote.NewGateway` after the Engine is ready and close it before Engine shutdown.

Expose an application controller:

```go
type RemoteController interface {
    Status(context.Context) (remote.FunnelStatus, error)
    Enable(context.Context) (remote.FunnelStatus, error)
    Disable(context.Context) error
    IssuePairing(context.Context) (remote.PairingOffer, error)
}
```

`IssuePairing` must require enabled Funnel status and use `<endpoint>/ws` as the offer endpoint.

- [ ] **Step 4: Add local admin endpoints and QR output**

Use one shared helper for loopback/admin-token checks. Add `github.com/skip2/go-qrcode` and encode this URI as a PNG:

```text
codeafar://pair?payload=<base64url(JSON({version:1,endpoint,code,expiresAt}))>
```

The code remains inside the encoded QR payload and request body; it must not be sent as an HTTP query to the remote gateway. Return PNG bytes as base64 in the authenticated local response with `Cache-Control: no-store`.

- [ ] **Step 5: Add non-technical Mac controls**

Add a “鸿蒙远程访问” section with status text, enable/disable button, “生成配对二维码”, QR image, expiry text, endpoint copy action, and conflict/unavailable guidance. `admin.js` must never render raw CLI output. Require confirmation before disabling a live remote entry.

- [ ] **Step 6: Verify Mac UI and lifecycle**

Run:

```bash
go test ./cmd/mac-app ./pkg/desktop ./web -count=1
go test -race ./pkg/remote ./pkg/engine -count=1
node --check web/admin/admin.js
make mac-app verify-mac-app
```

Expected: PASS and the Mac admin view contains the remote section while the public gateway still returns 404 for admin paths.

- [ ] **Step 7: Commit the Mac experience**

```bash
git add cmd/mac-app pkg/desktop web go.mod go.sum
git commit -m "feat: add Harmony remote access to Mac"
```

---

### Task 6: Harmony Protocol Model and Codec

**Files:**
- Create: `harmony/entry/src/main/ets/protocol/Models.ets`
- Create: `harmony/entry/src/main/ets/protocol/Codec.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/ProtocolCodec.test.ets`
- Modify: `pkg/product/harmony_contract_test.go`

**Interfaces:**
- Produces: `decodeServerMessage(json: string): ServerMessage`, `encodeClientMessage(message: ClientMessage): string`, and typed Provider/Session/Project/History models.
- Consumes: `pkg/protocol/messages.go` field names and protocol version.

- [ ] **Step 1: Write failing codec tests from real protocol fixtures**

```ts
it('decodes providers and keeps permission semantics', 0, () => {
  const value = decodeServerMessage('{"type":"provider_list","providers":[{"id":"codex","name":"Codex","available":true,"permissions":[{"id":"readOnly","label":"只读","description":"只检查","mutable":true}]}]}')
  expect(value.type).assertEqual('provider_list')
  const providers = (value as ProviderListMessage).providers
  expect(providers[0].permissions[0].id).assertEqual('readOnly')
})

it('rejects messages without a type', 0, () => {
  expect(() => decodeServerMessage('{"content":"x"}')).assertThrow()
})
```

Add fixtures for `hello`, `session_list`, `session_created`, `history`, `token`, `tool_use`, `done`, `permission_changed`, `text_accepted`, and `error`; add outbound exact-JSON tests for auth, create/select/load/list/set-permission/text.

- [ ] **Step 2: Run Harmony tests and verify RED**

Run: `make harmony-test`  
Expected: FAIL because protocol files/types are missing.

- [ ] **Step 3: Define closed message unions**

Use explicit interfaces such as:

```ts
export interface ProviderPermission {
  id: string
  label: string
  description: string
  dangerous: boolean
  mutable: boolean
}

export interface SessionInfo {
  sessionId: string
  name: string
  status: string
  cwd: string
  provider: string
  model: string
  permissionMode: string
}

export type ServerMessage = HelloMessage | ProviderListMessage | SessionListMessage |
  ProjectListMessage | TemplateListMessage | SessionCreatedMessage | HistoryMessage |
  TokenMessage | ToolUseMessage | DoneMessage | PermissionChangedMessage |
  TextAcceptedMessage | ErrorMessage
```

No `any` type and no silent defaulting of required identifiers are allowed.

- [ ] **Step 4: Implement strict type dispatch**

Parse once, validate object/type, then dispatch to one decoder per message. Unknown future types return `UnknownMessage` for logging-free ignore; malformed known types throw `ProtocolError`. Outbound encoding uses the exact lower-camel-case field names from Go.

- [ ] **Step 5: Run protocol and cross-platform contract tests**

```bash
make harmony-test
go test ./pkg/product -run TestHarmony -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit protocol parity**

```bash
git add harmony/entry/src/main/ets/protocol harmony/entry/src/ohosTest/ets/test/ProtocolCodec.test.ets pkg/product/harmony_contract_test.go
git commit -m "feat: add Harmony protocol codec"
```

---

### Task 7: Secure Pairing, Credential Storage, and WSS Lifecycle

**Files:**
- Create: `harmony/entry/src/main/ets/security/CredentialStore.ets`
- Create: `harmony/entry/src/main/ets/networking/PairingClient.ets`
- Create: `harmony/entry/src/main/ets/networking/CodeAfarSocket.ets`
- Create: `harmony/entry/src/main/ets/stores/ConnectionStore.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/PairingClient.test.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/ConnectionStore.test.ets`
- Modify: `harmony/entry/src/main/module.json5`

**Interfaces:**
- Produces: `CredentialStore.load/save/clear`, `PairingClient.redeem(payload, deviceName)`, `CodeAfarSocket.connect/send/close`, and observable `ConnectionStore.state`.
- Consumes: Task 5 pairing JSON and Task 6 codec.

- [ ] **Step 1: Write failing pairing and reconnect tests with fakes**

Test rejection of non-`wss://` endpoints, expired QR payloads, unsupported payload versions, token persistence only after successful response, clearing on `DEVICE_NOT_AUTHORIZED`, auth as the first socket message, capped exponential delays `[1,2,4,8,15,15]`, foreground immediate retry, and background cancellation.

```ts
it('never downgrades an invalid WSS endpoint', 0, async () => {
  const client = new PairingClient(new FakeHTTP(), new FakeCredentialStore())
  await expectReject(client.redeem({ version: 1, endpoint: 'ws://host/ws', code: 'x', expiresAt: future }, 'Harmony'))
})
```

- [ ] **Step 2: Run Harmony tests and verify RED**

Run: `make harmony-test`  
Expected: FAIL for missing networking/security classes.

- [ ] **Step 3: Implement Asset Store credentials**

Store one JSON value keyed by alias `codeafar.remote.credential` through Asset Store Kit:

```ts
export interface StoredCredential {
  endpoint: string
  deviceToken: string
  deviceName: string
}
```

Never mirror the token into AppStorage, preferences, navigation parameters, console output, or error messages. Query returns `null` when absent; save replaces the prior credential atomically; clear deletes it.

- [ ] **Step 4: Implement pairing and socket adapters**

`PairingClient` decodes the base64url QR payload locally, validates version/expiry/WSS, and POSTs `{ code, deviceName }` as JSON to the HTTPS form of the endpoint's origin plus `/pair`. `CodeAfarSocket` uses Network Kit WebSocket, sends auth on open, emits typed messages, applies the reconnect schedule, and stops retrying on TLS/protocol/auth errors.

- [ ] **Step 5: Map technical errors to closed user states**

```ts
export type ConnectionState = 'unpaired' | 'connecting' | 'connected' |
  'macOffline' | 'needsPairing' | 'protocolMismatch' | 'tlsError'
```

Raw exception text is retained only in an in-memory debug value excluded from UI and logs.

- [ ] **Step 6: Run tests and build**

```bash
make harmony-test
make harmony-hap
```

Expected: PASS and no credential pattern in the HAP staging tree (`! rg 'dt_' harmony/entry/build`).

- [ ] **Step 7: Commit secure connectivity**

```bash
git add harmony/entry/src/main/ets/security harmony/entry/src/main/ets/networking harmony/entry/src/main/ets/stores/ConnectionStore.ets harmony/entry/src/ohosTest harmony/entry/src/main/module.json5
git commit -m "feat: connect Harmony client securely"
```

---

### Task 8: Provider, Session, and Chat Stores

**Files:**
- Create: `harmony/entry/src/main/ets/stores/SessionStore.ets`
- Create: `harmony/entry/src/main/ets/stores/ChatStore.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/SessionStore.test.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/ChatStore.test.ets`
- Modify: `pkg/product/harmony_contract_test.go`

**Interfaces:**
- Produces: `SessionStore.activeProvider`, `visibleSessions`, `switchProvider`, `beginDraft`, `selectSession`; `ChatStore.composer`, `messages`, `send`, `handle`.
- Consumes: Task 6 typed messages and Task 7 socket sender.

- [ ] **Step 1: Write failing workspace and transaction tests**

Cover: filter sessions by provider, remember one session ID per provider, switch to draft when no remembered session, prevent cross-provider selected session, fallback when active provider becomes unavailable, derive permissions from provider descriptors, preserve composer across disconnect, ignore `tool_use`, merge adjacent Assistant tokens, and bound rendered history.

Add the exact first-message test:

```ts
it('creates once and sends first text after matching session_created', 0, () => {
  const socket = new FakeSender()
  const chat = new ChatStore(socket, () => 'req-1')
  chat.beginDraft('/project', 'codex', 'workspaceWrite')
  chat.composer = '检查项目'
  chat.send()
  expect(socket.sent[0]).assertDeepEquals({ type: 'control', action: 'create_session', requestId: 'req-1', workingDir: '/project', provider: 'codex', permissionMode: 'workspaceWrite', name: '检查项目' })
  chat.handle(sessionCreated('other'))
  expect(socket.sent.length).assertEqual(1)
  chat.handle(sessionCreated('req-1'))
  expect(socket.sent[1]).assertDeepEquals({ type: 'text', content: '检查项目', requestId: 'req-1' })
})
```

- [ ] **Step 2: Run tests and verify RED**

Run: `make harmony-test`  
Expected: FAIL for missing stores.

- [ ] **Step 3: Implement SessionStore invariants**

Persist only `activeProvider` and `lastSessionIds` in ordinary preferences; no token, chat, prompt, or project path. On provider/session lists, reconcile IDs, then restore. `switchProvider` saves the old selected session before changing provider and never mutates an existing session's provider.

- [ ] **Step 4: Implement ChatStore message handling**

Use a discriminated switch. `tool_use` is an explicit no-op. History renders only `text`, `token`, and `done`. First-message state contains `{requestId, content, displayed, sessionId}` until `text_accepted`; reconnect resends the create/text phase required by current state with the same request ID.

- [ ] **Step 5: Run store and contract tests**

```bash
make harmony-test
go test ./pkg/product -run 'TestHarmony|TestTool' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit state behavior**

```bash
git add harmony/entry/src/main/ets/stores harmony/entry/src/ohosTest/ets/test/SessionStore.test.ets harmony/entry/src/ohosTest/ets/test/ChatStore.test.ets pkg/product/harmony_contract_test.go
git commit -m "feat: add Harmony session workspaces"
```

---

### Task 9: Native ArkUI Pairing, Session, and Chat Experience

**Files:**
- Modify: `harmony/entry/src/main/ets/pages/Index.ets`
- Create: `harmony/entry/src/main/ets/pages/PairingPage.ets`
- Create: `harmony/entry/src/main/ets/pages/SessionPage.ets`
- Create: `harmony/entry/src/main/ets/pages/ChatPage.ets`
- Create: `harmony/entry/src/main/ets/pages/SettingsPage.ets`
- Create: `harmony/entry/src/main/ets/components/ProviderToolbar.ets`
- Create: `harmony/entry/src/main/ets/components/SessionList.ets`
- Create: `harmony/entry/src/main/ets/components/NewSessionSheet.ets`
- Create: `harmony/entry/src/main/ets/components/MessageBubble.ets`
- Create: `harmony/entry/src/main/ets/components/Composer.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/WorkspaceUI.test.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/ChatUI.test.ets`

**Interfaces:**
- Produces: the complete phone/tablet native experience.
- Consumes: Tasks 7–8 stores and actions.

- [ ] **Step 1: Write failing component/UI tests**

Use stable component keys: `provider-claude`, `provider-codex`, `new-session`, `session-list`, `project-picker`, `permission-picker`, `composer`, `voice`, `send`, `connection-state`. Test same-row provider/+ layout, provider-filtered histories, new-session sheet options, bottom composer, disabled send for blank content, long-press selection/copy action, user-facing error copy, and absence of a tool bubble component.

- [ ] **Step 2: Run UI tests and verify RED**

Run: `make harmony-test`  
Expected: FAIL because pages/components do not exist.

- [ ] **Step 3: Implement pairing and navigation**

PairingPage provides Scan Kit QR input plus a text-paste fallback for simulator tests. It shows only `扫描 Mac 配对码`, `正在配对`, `配对码已失效`, `无法连接 Mac`, and `重新配对` states. Successful pairing replaces the navigation stack with SessionPage; settings can clear the credential after confirmation.

- [ ] **Step 4: Implement provider/session adaptive layout**

`ProviderToolbar` places segmented Claude/Codex controls and `+` in one row. Phone uses navigation from session list to chat. When available width is at least 840 vp, `SessionPage` renders a fixed 300 vp sidebar and flexible chat pane. Both paths use the same stores and test keys.

- [ ] **Step 5: Implement new-session and chat components**

`NewSessionSheet` lists current `projects` and `activeProviderInfo.permissions`; it has no Provider picker. `MessageBubble` exposes selectable text and a copy button. `Composer` is bottom-aligned, uses the keyboard send action for submit, preserves explicit newline insertion, and calls the same `ChatStore.send()` as the send button.

- [ ] **Step 6: Run tests, build, and inspect screenshots**

```bash
make harmony-test
make harmony-hap
make harmony-validate
```

Launch the phone emulator at its default portrait size and a tablet emulator/preview at width >= 840 vp. Capture pairing, session, draft sheet, and chat screenshots under `build/qa/harmony/` and visually verify spacing, truncation, keyboard avoidance, contrast, and touch target sizes.

- [ ] **Step 7: Commit the native UI**

```bash
git add harmony/entry/src/main/ets/pages harmony/entry/src/main/ets/components harmony/entry/src/ohosTest
git commit -m "feat: build native Harmony chat UI"
```

---

### Task 10: Strict On-Device Speech Policy

**Files:**
- Create: `harmony/entry/src/main/ets/speech/SpeechController.ets`
- Create: `harmony/entry/src/main/ets/speech/SystemSpeechEngine.ets`
- Create: `harmony/entry/src/ohosTest/ets/test/SpeechController.test.ets`
- Modify: `harmony/entry/src/main/ets/components/Composer.ets`
- Modify: `harmony/entry/src/main/ets/entryability/EntryAbility.ets`
- Modify: `harmony/entry/src/main/module.json5`
- Modify: `pkg/product/voice_contract_test.go`

**Interfaces:**
- Produces: `SpeechController.toggle(baseDraft)`, `stop()`, `state`, and `onText(text)`.
- Consumes: an injectable `OnDeviceSpeechEngine` whose `capability()` must return an explicit verified guarantee.

- [ ] **Step 1: Write failing policy and lifecycle tests**

```ts
it('refuses an engine without explicit on-device guarantee', 0, async () => {
  const engine = new FakeSpeechEngine({ available: true, onDeviceGuaranteed: false })
  const speech = new SpeechController(engine, async () => true)
  await speech.toggle('原有文字')
  expect(speech.state.kind).assertEqual('unavailable')
  expect(engine.startCount).assertEqual(0)
})
```

Also test permission denial, partial replacement without overwriting base text, final text not calling ChatStore.send, double-toggle stop, app background stop, error cleanup, and blank result.

- [ ] **Step 2: Run voice tests and verify RED**

Run: `make harmony-test && go test ./pkg/product -run TestVoice -count=1`  
Expected: FAIL for missing Harmony speech policy markers.

- [ ] **Step 3: Audit the installed SDK before selecting production behavior**

Search only official installed SDK declarations and Huawei documentation for an API field or mode that explicitly guarantees offline/on-device speech. Record the exact symbol and API version in a code comment and `docs/testing/harmony-v1-acceptance-plan.md`.

If such a guarantee exists, `SystemSpeechEngine.capability()` returns true only when both the API mode and current locale/device support it. If no explicit guarantee exists, production `SystemSpeechEngine` returns `{ available: false, onDeviceGuaranteed: false }`; the microphone remains visible but disabled with `当前设备或语言不支持离线语音输入`.

- [ ] **Step 4: Implement controller and lifecycle cleanup**

```ts
export interface SpeechCapability {
  available: boolean
  onDeviceGuaranteed: boolean
}

export interface OnDeviceSpeechEngine {
  capability(): Promise<SpeechCapability>
  start(onPartial: (text: string) => void, onFinal: (text: string) => void, onError: () => void): Promise<void>
  stop(): Promise<void>
}
```

The controller stores `baseDraft`, appends one natural separator, replaces only the active speech segment, and stops on non-active app lifecycle state.

- [ ] **Step 5: Run policy, lifecycle, and build checks**

```bash
make harmony-test
go test ./pkg/product -run TestVoice -count=1
make harmony-hap
```

Expected: PASS; no cloud speech URL, API key, or automatic send call exists in `harmony/`.

- [ ] **Step 6: Commit speech privacy behavior**

```bash
git add harmony/entry/src/main/ets/speech harmony/entry/src/main/ets/components/Composer.ets harmony/entry/src/main/ets/entryability/EntryAbility.ets harmony/entry/src/main/module.json5 harmony/entry/src/ohosTest pkg/product/voice_contract_test.go
git commit -m "feat: gate Harmony speech on device"
```

---

### Task 11: Real Funnel Integration, HAP Acceptance, and Documentation

**Files:**
- Create: `docs/testing/harmony-v1-acceptance-plan.md`
- Create: `scripts/test-harmony-contract.sh`
- Modify: `README.md`
- Modify: `docs/TESTING.md`
- Modify: `Makefile`
- Modify: `scripts/package-release.sh`
- Modify: `pkg/product/harmony_contract_test.go`

**Interfaces:**
- Produces: a repeatable full verification command and packaged Harmony HAP.
- Consumes: every prior task.

- [ ] **Step 1: Write the failing release/acceptance contract**

Require `make verify` or a new `make verify-harmony` path to run repository tests, Harmony validation/tests/HAP build, remote gateway security tests, and a credential scan. Require release packaging to copy the signed/debug HAP into `build/release/` and include it in `SHA256SUMS`.

- [ ] **Step 2: Run the contract and verify RED**

Run: `go test ./pkg/product -run TestHarmony -count=1`  
Expected: FAIL for missing acceptance script, documentation, or release markers.

- [ ] **Step 3: Create the full verification script**

`scripts/test-harmony-contract.sh` must run:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go test ./pkg/remote ./pkg/engine ./pkg/desktop ./pkg/product -count=1
go test -race ./pkg/remote ./pkg/engine -count=1
./scripts/validate-harmony-project.sh
./scripts/test-harmony.sh
! rg -n 'ws://|tskey-auth-|dt_[A-Za-z0-9_-]{16,}|deviceToken\s*:' harmony --glob '!**/*test*'
echo "HarmonyOS contract OK"
```

- [ ] **Step 4: Run real Mac Funnel acceptance**

From the installed Mac app, explicitly enable “鸿蒙远程访问”. Verify with read-only status and HTTPS checks:

```bash
tailscale funnel status --json
funnel_host="$(tailscale status --json | jq -r '.Self.DNSName | rtrimstr(".")')"
funnel_base="https://${funnel_host}:8443"
curl --fail --silent --show-error "${funnel_base}/healthz"
test "$(curl -s -o /dev/null -w '%{http_code}' "${funnel_base}/admin/status")" = 404
test "$(curl -s -o /dev/null -w '%{http_code}' "${funnel_base}/desktop/projects")" = 404
```

Use the generated one-time QR payload through the Harmony client, verify second redemption fails, connect WSS, then revoke the device in Mac admin and verify immediate disconnect/re-pair state. Disable remote access and verify the Funnel endpoint closes without altering unrelated Tailscale configuration.

- [ ] **Step 5: Run complete Harmony simulator acceptance**

Install the HAP through DevEco/hdc, then execute the matrix in `docs/testing/harmony-v1-acceptance-plan.md`: pairing input, Claude/Codex history isolation, project/permission draft, duplicate-safe first message, streaming, copy, newline/send, hidden tools, reconnect, token revoke, and user-facing errors. Save screenshots and logs with secrets redacted under ignored `build/qa/harmony/`.

- [ ] **Step 6: Run real-device-only checks when a device is available**

On a HarmonyOS NEXT phone, verify camera QR scan, WSS over cellular, network switching, lock/background resume, keyboard avoidance, clipboard, microphone permission, on-device capability gate, and no retained microphone session. If no physical device is available, mark only these hardware rows `BLOCKED: physical HarmonyOS NEXT device unavailable`; do not describe them as passed.

- [ ] **Step 7: Update documentation and package the HAP**

Document DevEco/SDK versions, build commands, Funnel requirements, pairing, device revocation, voice privacy, limitations, and troubleshooting. Add the HAP to release output and regenerate checksums.

- [ ] **Step 8: Run fresh final verification**

```bash
make verify
make verify-harmony
(cd android && ./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon)
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcodebuild test -quiet \
  -project ios/ClaudePhone.xcodeproj -scheme ClaudePhone \
  -destination 'platform=iOS Simulator,id=73BBB2BB-1053-4C85-A153-A2A7FB47D810' \
  CODE_SIGNING_ALLOWED=NO
make install-mac-app
./scripts/test-mac-reopen.sh
git diff --check
git status --short
```

Expected: every command exits 0, the worktree is clean after the final commit, `/Applications/CodeAfar.app` runs, and the packaged HAP checksum verifies.

- [ ] **Step 9: Commit the release-complete delivery**

```bash
git add README.md docs/TESTING.md docs/testing/harmony-v1-acceptance-plan.md scripts Makefile pkg/product/harmony_contract_test.go
git commit -m "docs: complete HarmonyOS delivery"
```

---

## Final Review Checklist

- [ ] Every remote public path outside `/pair`, `/ws`, and `/healthz` returns 404.
- [ ] Pairing codes expire, cannot replay, and do not appear in URL queries or logs.
- [ ] Device revocation closes live connections and survives Engine restart.
- [ ] Funnel configuration conflicts are non-destructive and disabling removes only CodeAfar ownership.
- [ ] Harmony credentials exist only in secure Asset Store.
- [ ] Claude/Codex histories, permissions, and restoration remain isolated.
- [ ] Tool activity is absent from live and historical ordinary chat.
- [ ] Speech cannot start without an explicit official on-device guarantee.
- [ ] Full Go, race, Web, Android, iOS, Harmony tests and HAP build pass.
- [ ] Mac app is reinstalled from the final commit and remote access can be disabled cleanly.
