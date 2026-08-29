# 决策：暂停 HarmonyOS NEXT 客户端（2026-08-29）

## 背景

2026-07-22 完成鸿蒙客户端设计（`docs/superpowers/specs/2026-07-22-harmonyos-client-design.md`）
与实施计划（`docs/superpowers/plans/2026-07-22-harmonyos-client.md`），随后在
`codex/harmonyos-client` 分支完成 11 个任务的实现：`harmony/` 完整 ArkTS 工程、
Mac 侧远程网关（`pkg/remote/` 配对 + 网关 + Funnel 控制）、15 个 Hypium 测试套件，
共 43 个提交。Go 侧（remote / engine / product）测试全绿。

## 暂停原因

1. **无 HarmonyOS NEXT 真实设备**：现有手机为 HarmonyOS 4.2（基于 AOSP 的上一代
   系统），无法安装 HAP；HAP 仅支持 NEXT（5.0 / API 12+）。真实设备验收项
   （扫码配对、麦克风语音、Funnel 实链路、前后台生命周期）无法执行。
2. **HAP 构建验收受阻**：本机 DevEco Studio 自带 API 26 SDK，工程锁定
   `compatibleSdkVersion 5.0.0(12)`，需另装 API 12 SDK 及模拟器镜像；磁盘与
   工具链成本与当前无设备的现实叠加，性价比不足。

HAP 构建、模拟器运行、真实设备验收三项在 2026-07-22 验收记录中本就标记为
BLOCKED（见归档分支 `build/qa/harmony/acceptance-summary.md`），暂停决定不改变
当时的验收结论。

## 重启路径

```bash
git worktree add .worktrees/harmonyos-client codex/harmonyos-client
```

分支头部 `b527b25`（含最后一批未提交加固改动，已在归档前收拢提交）。重启条件：

- 拿到一台 HarmonyOS NEXT（5.x）真实设备，或确定仅以模拟器验收交付；
- 在 DevEco Studio SDK Manager 补装 API 12 SDK；
- 复核 `harmony/hvigorw` 的 SDK 探测路径与 DevEco 新版目录结构是否仍匹配。

## 范围澄清

暂停仅限鸿蒙客户端。分支上的 Mac 侧远程网关 / Funnel / 配对代码为鸿蒙专用，
master 不含任何鸿蒙代码；iOS、Android、Mac 主线不受影响。

## 仓库更名备忘

同日将 GitHub 仓库由 `claude-phone` 更名为 `codeafar`（远程已重定向，本地
remote 已更新）。Go module 路径仍为 `github.com/yang-bin-free/claude-phone`
（43 文件 78 处引用），计划待主线稳定后随一次机械提交统一更名。
