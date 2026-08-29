# CodeAfar 自定义 CLI 命令配置设计

**日期：** 2026-08-29
**状态：** 已确认
**范围：** Mac 应用（engine / session / desktop / admin UI）；不改协议、不改 iOS/Android

## 1. 目标

让用户在 Mac 应用的设置页配置 Claude/Codex 的启动命令，命令支持「可执行文件 + 参数」的形式（例如 `dcc claude`、`/usr/local/bin/my-wrapper --profile work`），配置持久化到 `~/.codeafar/config.yaml`，重启应用后生效。

动机：CLI 包装器（企业通道、代理切换、个人统计等）是真实需求；当前 bin 只能通过命令行 flag 指定，Finder 启动的用户没有任何入口，且解析层不支持带参数的命令。

## 2. 非目标

- 不探测、不命名、不集成任何特定包装器（如 DCC）。仓库保持中立；README 仅说明「可配置启动命令」这一通用能力，不提及任何具体包装器。
- 不做运行时热切换。修改命令后需要重启应用；在途会话不受影响。
- 不改 `pkg/protocol`、iOS/Android 客户端、远程网关。
- 不支持带引号的参数（参数内含空格）。命令用空白拆分；含引号或参数内空格的输入在校验时被拒绝并提示。

## 3. 已验证的前提

- `dcc claude` 与 `claude` 共用 `~/.claude/projects/` 转录目录，`--session-id` 透传，stream-json 输入输出完全兼容（2026-08-29 实测，含 `system/init`、`stream_event/text_delta`、stdin 流式喂 user 消息、固定 session-id 的 transcript 落盘）。
- `dcc claude --version` 输出 `2.1.245 (Claude Code)`，可被现有 `cliVersionPattern` 解析。
- dcc 注入的 `hook_started`/`hook_response` 事件流与启动横幅（非 JSON 行）都会被现有逐行 JSON 解析路径静默忽略，无需改动。
- 因此会话恢复（`ClaudeSessionExists` 的转录检查）、历史记录在换用包装命令后照常工作。

## 4. 数据流

```text
设置页(admin.js) ──PATCH /admin/settings──▶ updateRuntimeConfig()
     │  新增字段 claudeCommand / codexCommand
     │                                          ├─ 校验（见 §5）
     │                                          └─ 原子写 ~/.codeafar/config.yaml
     ▼
重启 Mac 应用
     │
main.go 读 config.yaml → engine.Config 的 ClaudeBin/CodexBin
     │  （命令行 flag 显式设置时优先于文件值，保持开发者习惯）
     ▼
resolveCodingAgentBinary("dcc claude")
     │  拆词 → 对第一个词执行现有搜寻（PATH、~/.local/bin、volta、asdf、mise、
     │  nvm、homebrew）→ ResolvedCommand{Path, PrependArgs}
     ▼
DetectCLIVersion(命令) → `<命令> --version`
     ▼
provider adapter → SessionConfig → exec.Command(path, prependArgs..., 原有参数...)
```

## 5. 配置与校验

- `runtimeConfig` 增加 `ClaudeCommand`/`CodexCommand`（yaml：`claudeCommand`/`codexCommand`）。旧 `config.yaml` 无此字段 → 零值 → 默认 `claude`/`codex`，向后兼容。
- 校验在 `updateRuntimeConfig` 保存时执行，失败返回 400 阻止保存（严格模式，理由：错误命令会让 provider 启动即不可用，早失败优于运行时排查）：
  - 非空值按空白拆分，1 ≤ 词数 ≤ 8，总长度 ≤ 200；
  - 不得包含引号字符；
  - 第一个词必须解析到可执行文件（复用现有搜寻逻辑）；
  - 空字符串合法 = 恢复默认。
- 命令行 flag 与配置文件的关系：flag 显式设置（`fs.Visit` 判断）> config.yaml > 默认值。

## 6. 改动点

### A. 命令解析（pkg/desktop/claude.go）

- `ResolveClaudeBinary`/`ResolveCodexBinary` 返回 `ResolvedCommand{Path string; PrependArgs []string}`；`resolveCodingAgentBinary` 接受多词命令：`strings.Fields` 拆分，第一个词走现有搜寻，其余词为前置参数。
- 单词命令行为与现状完全一致（PrependArgs 为空）。

### B. 执行层（pkg/session、pkg/provider、pkg/engine）

- `session.ClaudeConfig`/`CodexConfig` 增加 `BinArgs []string`；`exec.Command(bin, ...)` 变为 `exec.Command(bin, append(binArgs, args...)...)`。
- provider adapter 构造函数接受命令（bin + 前置参数），`SessionConfig` 相应扩展。
- `DetectCLIVersion` 接受命令（路径 + 前置参数）执行 `--version`。

### C. 配置与 UI（pkg/engine/runtime_config.go、admin.go、web/admin）

- `runtimeConfig`、`UpdateSettingsRequest`、`adminproto.AgentStatus`（`ClaudeBin`/`CodexBin` 显示完整命令字符串）按 §5 扩展。
- admin 设置页增加两个文本输入框（Claude 命令 / Codex 命令），placeholder 显示当前生效值，保存走现有 PATCH 链路，反馈文案与现有风格一致。

### D. 启动装配（cmd/mac-app）

- `main.go` 在 flag 未显式设置时读取 config.yaml 的命令字段构造 engine Config。
- `application.go` 的 `resolveProvider`/`detectVersion` 适配 `ResolvedCommand`。

## 7. 错误处理

- 保存时校验失败：400 + 用户可读原因（「命令不存在」「参数过多」等），不落盘。
- 启动时命令失效（保存后用户删了包装器）：现有 provider 不可用链路生效（Descriptor.Available=false + UnavailableReason），聊天页 Provider 入口禁用并显示原因——与现在 claude 缺失时的行为一致。
- 版本探测失败：现有 `detectVersion` 失败路径（provider 标记不可用）不变。

## 8. 测试与验收

- 单元测试：多词拆分、带绝对路径的多词、引号拒绝、不存在命令拒绝、超限拒绝、空值恢复默认、BinArgs 前置（假 CLI 脚本 e2e：输出合法 stream-json 的 shell 脚本验证一轮完整会话）。
- UI 验收（Playwright 真实浏览器，遵守项目铁律）：设置页填入命令 → 提交 → 断言 204 与 config.yaml 内容 → 刷新回读 → 重启 Mac app 进程 → 聊天页发消息走自定义命令（用假 CLI 脚本，不依赖外部登录态）→ 截图留证。
- 真实包装器端到端（本地人工验证，不进 CI、不进仓库）：配置 `dcc claude` 后真实跑一个会话确认流式输出与历史恢复。

## 9. 参考依据

- `pkg/provider/provider.go`：Adapter/Registry/SessionConfig 边界。
- `pkg/engine/config.go`、`runtime_config.go`：配置流与原子写。
- `pkg/desktop/claude.go`：现有 CLI 搜寻逻辑（本次仅扩展第一个词语义）。
- 2026-08-29 DCC 透传实测记录（会话内）：协议、转录目录、版本输出兼容性。
