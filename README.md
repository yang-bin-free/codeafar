# CodeAfar

CodeAfar 是一个从电脑或手机远程使用本机编程 Agent 的客户端。Mac V1 当前支持 Claude Code 和 Codex，并以同一套会话、目录授权和聊天体验承载两者。

它的核心价值是把语音转写、会话状态、目录授权和 UI 留在本地，只把最终确认的提示词交给编程 Agent，减少无效上下文和 token 消耗。

## Mac V1

当前能力：

- 在新会话草稿中选择项目目录、Provider 和权限模式；发送第一条消息时才创建会话。
- 通过原生 Finder 目录选择器授权项目，不接受网页伪造的任意路径。
- Return 发送，Shift+Return 换行；聊天内容可选择和复制。
- Claude/Codex 回复、工具操作和错误以统一格式显示，会话与历史重启后恢复。
- 会话中可调整权限模式；忙碌时延后到当前轮结束，空闲时安全重启并恢复上下文。
- 菜单栏常驻、关窗隐藏、再次打开恢复窗口、暂停/恢复、诊断和开机自启。
- 首次启动把旧 `~/.claude-phone` 数据迁移到 `~/.codeafar`，不覆盖已经存在的新数据。

Claude 权限模式：

- `严格`（`default`）：Claude 在执行需要授权的工具操作前询问，适合作为默认值。
- `审阅`（`acceptEdits`）：自动接受文件编辑，其他敏感操作仍按 Claude 的规则处理。
- `信任`（`bypassPermissions`）：跳过权限确认，只应在完全信任的目录中使用。

Codex 权限模式：

- `只读`（`readOnly`）：可检查和分析项目，但不能修改文件。
- `工作区访问`（`workspaceWrite`）：可在当前项目内修改文件和执行命令。
- `完全访问`（`fullAccess`）：关闭 Codex 文件系统和网络沙箱，只应在完全信任的环境中使用。

Provider 在创建会话后固定；需要切换时创建一个新会话。Codex 非交互模式无法弹出临时批准，因此 CodeAfar 在创建会话时一次性选择沙箱，并以 `never` 批准策略运行；超出权限的操作会失败并显示原因。

### 自定义启动命令

设置页可以把 Claude/Codex 的启动命令改成任意「可执行文件 + 参数」的形式（例如本地的包装脚本）。命令在保存时校验：第一个词必须是可执行文件，最多 8 个参数、200 个字符，不支持引号参数。修改在重启应用后生效；会话历史与恢复不受影响。

## 安装并打开 Mac 应用

要求：macOS 12 或更高版本，以及至少一个已经可用并登录的编码 Agent CLI：

```bash
claude --version
# 或
codex login
codex --version
```

```bash
make install-mac-app
```

命令会构建、签名校验并原子安装到 `/Applications/CodeAfar.app`，然后启动应用。如果新版本无法启动，会恢复原安装。也可以直接在 Finder 的“应用程序”中双击 CodeAfar。

仅构建发布包：

```bash
VERSION=0.1.0-dev make mac-release
shasum -a 256 -c build/release/SHA256SUMS
```

产物是 `build/CodeAfar.app` 和 `build/release/codeafar-macos-<version>.zip`。仓库构建使用 ad-hoc 签名，公开分发前仍需要 Developer ID 签名和 Apple 公证。

无界面模式：

```bash
make build-agent
./build/codeafar-agent serve
./build/codeafar-agent key --name Pixel
```

## 会话设计

点击“新会话”只创建一个未提交草稿。用户从 Finder 选目录，按需修改 Provider 和权限，然后输入消息；第一次发送会以稳定的 `requestId` 创建会话，收到 `session_created` 后发送带同一消息 ID 的文本，并保留到服务端返回 `text_accepted`。断线后重试会由服务端去重，避免重复会话或重复执行首条消息。

当前 Provider：

| Provider | 状态 | 权限模式 |
|---|---|---|
| Claude Code | 可用 | 严格、审阅、信任、计划 |
| Codex | 可用 | 只读、工作区访问、完全访问 |

应用会分别检测两个 CLI。只有其中一个可用时仍可正常启动；缺失的 Provider 会在选择器中显示为不可用并给出原因。

## 本地语音输入

语音只负责填写消息草稿，不会自动发送；识别结果追加到原有文字，用户可以继续修改。

- Android 12+：只使用 `createOnDeviceSpeechRecognizer`。设备没有端侧识别服务时明确提示不可用，不回退到网络识别。
- iOS 26+：使用 `SpeechAnalyzer`、`SpeechTranscriber` 和 Apple 管理的本地语言资源。
- iOS 18–25：只在 `SFSpeechRecognizer.supportsOnDeviceRecognition` 可用时启动，并强制 `requiresOnDeviceRecognition = true`。

系统可能需要下载由 Apple/Android 系统管理的语言资源；这是一次性的模型资源准备，不代表语音内容发送到 CodeAfar 或 Agent 服务端。

## 开发与验证

```bash
make verify
make mac-release

cd android
JAVA_HOME=/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home \
ANDROID_HOME="$HOME/Library/Android/sdk" \
./gradlew :app:testDebugUnitTest :app:assembleDebug --no-daemon

cd ..
xcodebuild test -quiet \
  -project ios/ClaudePhone.xcodeproj \
  -scheme ClaudePhone \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
  CODE_SIGNING_ALLOWED=NO
```

完整 Mac 验收矩阵见 [`docs/testing/mac-v1-acceptance-plan.md`](docs/testing/mac-v1-acceptance-plan.md)。

## 架构

```text
Mac CodeAfar.app
  ├─ Cocoa/WebKit 桌面壳与本地管理页
  ├─ Go WebSocket/HTTP 引擎
  ├─ Provider Registry
  │    ├─ Claude Code adapter → 本机 claude CLI
  │    └─ Codex adapter → 本机 codex CLI（JSONL + thread resume）
  └─ ~/.codeafar（项目、设备、会话、权限、历史）

Android / iOS
  ├─ 原生 VPN/Tailscale 接入
  ├─ 共享会话协议
  └─ 严格端侧语音 → 可编辑消息草稿
```

主要目录：

- `cmd/mac-app`：Mac 桌面入口。
- `cmd/mac-agent`：Mac 无头 Agent。
- `pkg/engine`：连接、会话、持久化和权限控制。
- `pkg/provider`：Provider 描述与适配器注册表。
- `pkg/desktop`：仅回环开放的桌面服务和原生桥接。
- `web/chat`：Mac/Android 共享聊天界面。
- `android`、`ios`：移动端原生壳与本地语音。

## 安全与隐私

- 桌面管理接口只监听 loopback，并校验进程随机管理 token。
- 手机连接使用 Device Token；跨网络链路由 Tailscale/WireGuard 加密。
- Finder 选择的项目才会加入桌面授权列表。
- token、Auth Key 和语音内容不会写入仓库。
- Claude `信任`模式和 Codex `完全访问`模式都会放宽 Agent 自身的限制；CodeAfar 会清晰展示当前模式，但无法替代操作系统沙箱。

## License

MIT。第三方许可证见 `THIRD_PARTY_LICENSES.md`。
