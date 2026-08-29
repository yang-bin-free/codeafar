# Mac V1 Acceptance Plan

## Release rule

Mac V1 is releasable only when every `P0` and `P1` case below passes on the
packaged application, `make verify` passes, the installed bundle passes
`codesign --verify --deep --strict`, and no CodeAfar or `caffeinate` child
process is left behind after Quit. A failed case reopens the release: fix it,
add a regression test, rebuild, and rerun the entire plan from a clean install.

## Test environment

- macOS 12 or newer, Apple Silicon or Intel as available.
- Packaged `CodeAfar.app`, not an unbundled Go executable.
- Real installed Claude Code and Codex for provider end-to-end responses;
  deterministic fake CLIs for streaming, resume and failure injection.
- Isolated data directory for destructive tests. Existing `~/.codeafar`
  data is never deleted or overwritten.
- Autostart state is captured before testing and restored afterward.

## Acceptance matrix

| ID | Priority | Scenario | Pass criteria | Evidence |
|---|---|---|---|---|
| PKG-01 | P0 | Full repository verification | Unit, integration, race, JS, Android contract and iOS structure checks exit 0 | `make verify` output |
| PKG-02 | P0 | Build and package Mac app | Bundle contains executable, icon and versioned plist; ZIP and SHA256 validate | build log, `codesign`, `shasum` |
| PKG-03 | P1 | Clean install in `/Applications` | Installed bundle launches from Finder and runs the packaged executable | process path and screenshot |
| LIFE-01 | P0 | First launch | Window appears, menu item appears, status becomes ready, no crash | window count, `/desktop/status` |
| LIFE-02 | P0 | Close then reopen | Closing hides the window; double-clicking the running app restores exactly one window | `scripts/test-mac-reopen.sh` |
| LIFE-03 | P1 | Repeated open | Repeated double-click/open does not spawn duplicate engines or listeners | PID and listener counts |
| LIFE-04 | P0 | Quit | Window, listener, Claude child and app-owned `caffeinate` terminate | process audit |
| MENU-01 | P1 | Show and Hide | Both menu commands change window visibility without stopping the engine | accessibility assertions |
| MENU-02 | P1 | Pause and Resume | Pause returns an unavailable state without exiting; Resume returns ready | status and menu text |
| MENU-03 | P1 | Diagnostics | Diagnostics opens the management view and all panels load | DOM/screenshot and console |
| MENU-04 | P1 | Autostart toggle | Enable creates a valid LaunchAgent; disable removes it; original state is restored | `launchctl` and file checks |
| CHAT-01 | P0 | New real-Claude session | Session starts in selected directory and real Claude returns the requested response | UI transcript and history file |
| CHAT-02 | P0 | Streaming | Partial text is rendered in order, once, without duplicated chunks | deterministic fake transcript |
| CHAT-03 | P1 | Stop session | Stop ends the Claude child, updates controls and does not affect the app engine | UI and process audit |
| CHAT-04 | P0 | Restart and restore | App restart restores session list, selected history and send readiness | before/after state |
| CHAT-05 | P1 | Multiple sessions | Switching sessions keeps histories isolated and selects the correct process | UI and persisted files |
| CHAT-06 | P1 | Long/multiline input | Unicode and multiline input round-trip without truncation or broken layout | transcript and screenshot |
| CHAT-07 | P0 | Codex multi-turn and restart | Codex tool event is readable, second turn retains context, and app restart resumes the persisted Codex thread | `CODEAFAR_REAL_CODEX=1` E2E and installed UI transcript |
| CHAT-08 | P1 | Provider-specific creation | New-session selector disables unavailable providers and switches to valid permissions for Claude/Codex; provider is fixed after creation | UI assertions and process args |
| ADMIN-01 | P1 | Project CRUD | Add/update/remove project persists and controls the new-session selector | admin UI and YAML |
| ADMIN-02 | P1 | Template CRUD | Add/update/remove template persists and inserts the correct prompt | admin UI and YAML |
| ADMIN-03 | P1 | Permission rules | Rule changes persist and are passed to newly created sessions | admin UI and process args/test |
| ADMIN-04 | P1 | Runtime settings | Valid settings apply; invalid directory, permission and limits are rejected | HTTP/UI response and YAML |
| ADMIN-06 | P1 | Custom launch commands | A multi-word command persists, is rejected when its first word is not an executable, and drives real turns after restart | `scripts/ui-test-cli-settings.js` browser run, screenshots and YAML |
| ADMIN-05 | P1 | Diagnostics content | Claude/Codex versions and paths, devices, sessions and health counts match runtime | UI vs process/status audit |
| ERR-01 | P0 | Claude CLI missing | App stays open and shows an actionable unavailable message; Resume can recover | screenshot and status |
| ERR-02 | P1 | Claude CLI exits early | User sees an error instead of a permanently busy session | UI and injected stderr |
| ERR-03 | P1 | Unauthorized browser client | Connection is rejected once with a stable message; no unbounded duplicate banners | DOM and console |
| ERR-04 | P1 | Port already occupied | Launch fails visibly and leaves no partial background process | exit/log/process audit |
| ERR-05 | P1 | Invalid/corrupt persisted data | App starts with a clear error or safe defaults and does not destroy the file | backup, UI/log and checksum |
| ERR-06 | P0 | One provider missing | App remains ready when either Claude or Codex is usable; the missing provider is visible but disabled with a reason | status, provider list and UI screenshot |
| SEC-01 | P0 | Loopback boundary | Desktop server binds only to loopback and rejects non-loopback configuration | listener and unit tests |
| SEC-02 | P0 | Admin/device authorization | Missing or wrong tokens cannot access WebSocket/admin endpoints | HTTP/WebSocket assertions |
| SEC-03 | P1 | Secret scan | No token, key or credential is present in tracked changes or release metadata | repository scan |
| UX-01 | P1 | Desktop layout | 1180×760 has no overlap, clipping or unusable controls in chat/admin states | screenshots |
| UX-02 | P2 | Narrow layout | Small viewport remains navigable and controls stay reachable | browser viewport screenshots |
| UX-03 | P1 | Console health | No uncaught JavaScript errors during the complete workflow | browser logs |

## Execution order

1. Record commit, OS, Claude version, current autostart state and process baseline.
2. Run package/build gates and install the resulting bundle.
3. Run lifecycle and menu cases against the installed application.
4. Run deterministic chat cases with fake Claude in an isolated data directory.
5. Run real Claude and Codex end-to-end prompts; for Codex, verify tool translation, multi-turn context and restart/resume.
6. Run administration and persisted-data cases.
7. Run failure injection and authorization cases.
8. Audit processes, restore autostart, verify the release ZIP, and rerun
   `make verify` after the last code change.
9. Publish the QA report with per-case results, issue evidence, fix commits and
   the exact final commit SHA.

## Required release artifacts

- `build/CodeAfar.app`
- `build/release/codeafar-macos-<version>.zip`
- `build/release/SHA256SUMS`
- `.gstack/qa-reports/qa-report-codeafar-mac-<date>.md`
