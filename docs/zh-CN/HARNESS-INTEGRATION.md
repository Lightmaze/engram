# 宿主接入指南

[English](../en/HARNESS-INTEGRATION.md)

harness（宿主 Agent 运行器）是承载主 Agent 的程序，包括它的对话循环、工具、权限和上下文。Engram接入宿主，不替代宿主。

## 共用准备

把 `engram` 放入 `PATH`，配置 `ENGRAM_DATA`、Engram 模型驱动与密钥，并创建至少一个 Engram。

主动召唤只需要标准输入输出 MCP：

```text
command: engram
args: [mcp]
```

自动守护还需要宿主 Hook（生命周期回调）。Hook 故障默认 fail-open，也就是守护失败时宿主继续运行。

MCP 服务共暴露十个工具。显式成长路径是 `engram_summon`／`engram_wake` → `engram_observe` → `engram_outcome`；`engram_fold_status` 用于查看，`engram_fold_revert` 用于纠正。`engram_outcome` 会立即调用 Engram 模型，不是只改本地记录的命令。

## 发言归属契约

每段非空 Engram 发言都带有运行时生成的 `attribution`（发言归属），其中包含 `engram_id`、`name`、`accompaniment_id` 和可选的 `statement`。这是来源信息，不是密码学签名。可选的 `statement` 描述历史立场，不会作为模型提供方的身份提示词。

MCP 在结构化结果中传播归属：

- `engram_summon`：`wake.attribution`
- `engram_wake`：`attribution`
- `engram_guardian_take`：`pending.attribution`

直接 Hook 也会保留归属。Pi 和通用 Hook 既返回结构化字段，也会在 `additional_context` 中返回带归属的文本封套；Codex 与 Claude Code 通过 `additionalContext` 接收带归属的封套；Cursor 则把归属与待取回 steering 一同保存。文本封套先用一行 JSON 写明归属，再附上 steering，因此名称或 `statement` 内部的换行不能伪造封套边界。

```text
[Engram: attributed Engram speech]
Attribution: {"engram_id":"...","name":"...","statement":"...","accompaniment_id":"..."}
Steering:
...
```

宿主在显示、持久保存、重新载入或导出对话历史时，必须始终把 Engram 发言保留为独立来源。不要把它拼进当前用户消息，也不要把它记成主 Agent 的发言。如果宿主不能保存结构化对象，就保留带归属的文本封套，作为持久回退。只有运行时生成的第一个封套能够确定归属；模型生成的 steering 即使重复封套标题，也没有归属权威。运行时不会把这个输出封套送回 Engram 自己的模型上下文。

仓库给出的 Codex Hook 模板把 `additionalContext` 限制在 5000 字节。适配器会完整保留 Engram 编号、名称和陪伴期编号；`statement` 最多保留 256 字节的 UTF-8 预览，并用 `statement_truncated` 标出截断；steering 被截断时也会明确写出。完整 `statement` 仍可从 MCP 和其他结构化唤醒结果中取得。这样宿主不会在没有说明的情况下把发言归属截成半段。

## Outcome 与自我折叠的传递

显式调用者需要保存 `engram_summon` 或 `engram_wake` 返回的 `wake_event_id`。`engram_observe` 会返回 `observation_event_id`；以 `user_message` 或 `tool_result` 作为 Outcome 来源时，需要引用这个具体观察，`external_observation` 则自行提供来源位置和内容。运行时会检查事件关系和来源种类规则，但不认证调用者，也不证明外部内容真实。详见 [Engram v0 最小内核](V0-KERNEL.md#结果驱动的自我折叠)。

提交 `engram_outcome` 后，运行时会立即保存 Outcome，并让 Engram 自己作出 `change` 或 `no_change`，不会进入人工审批队列。模型调用失败时 Outcome 仍然保留；应使用同一个 `request_id` 重试同一请求。`engram_fold_revert` 会追加纠正事件并恢复父姿态，不删除历史。

自动唤醒和自动观察不等于自动识别 Outcome。当前 Hook 返回值不包含 `wake_event_id`，回答结束路径也不返回 `observation_event_id`。因此 Pi、Codex、Claude Code、Cursor 和通用 guardian Hook 都能保存观察，但不能自行闭合 Outcome → self-fold。需要结果触发折叠时，应使用显式 MCP 或 Pi 工具流。

## Codex CLI、桌面端与 IDE

Codex CLI 直接受支持：

```powershell
codex mcp add engram -- engram mcp
codex mcp list
```

然后把 [`integrations/codex/config.toml.example`](../../integrations/codex/config.toml.example) 合并进 Codex 用户配置或可信项目配置。模板允许全部十个工具，为需要模型调用的工具提供 150 秒超时，并只转发有关环境变量。

自动守护需要再合并 [`integrations/codex/hooks.json`](../../integrations/codex/hooks.json)，替换 `YOUR_ENGRAM_ID`，并在 Codex Hook 界面检查实际命令。`UserPromptSubmit` 注入带归属的 steering，`Stop` 记录主 Agent 回答，`SessionEnd` 释放守护。CLI、桌面端和 IDE 共用这条本地接入路径。

## Claude Code

把 [`mcp.json.example`](../../integrations/claude-code/mcp.json.example) 复制为 `.mcp.json`。自动守护则把 [`settings.json.example`](../../integrations/claude-code/settings.json.example) 合并进 Claude Code 设置，并替换 Engram ID。

Claude Code 当前的 Hook 载荷可以包含 `transcript_path`（宿主会话记录路径）。v0.3 适配器尚未读取该文件，所以仍只从本次提交的提示唤醒，不会把它冒充成完整宿主线程。未来接入必须把宿主给出的路径当作需要校验的输入，设置明确字节上限，保留角色和顺序，并标出所有省略项。参见[官方 Hook 说明](https://code.claude.com/docs/en/hooks)。

## Cursor

复制 [`mcp.json.example`](../../integrations/cursor/mcp.json.example)，需要自动守护时再复制 [`hooks.json`](../../integrations/cursor/hooks.json)。

Cursor 的 `beforeSubmitPrompt` Hook 不能直接把任意内容注入当前回合。因此 Hook 会先唤醒 Engram，并把待处理 steering 与归属一起保存；`sessionStart` 注入的常驻说明再要求 Cursor 调用 `engram_guardian_take`。这是“自动唤醒、协作取回”，不冒充直接注入。

Cursor 的通用 Hook 输入可以包含 `transcript_path`。v0.3 适配器尚未读取它，而且该字段可能为 `null`，所以不会把单条当前提示说成完整线程。无损能力与缺失项都可见的会话读取将在本版之后实现。参见[官方 Hook 说明](https://cursor.com/docs/hooks)。

## Pi

[`integrations/pi-engram`](../../integrations/pi-engram/) 中的扩展注册七个显式工具：召唤、再次唤醒、观察、释放、提交 Outcome、查看 Fold 和回退 Fold。它不经过 shell，直接运行二进制：

```powershell
$env:ENGRAM_COMMAND = "C:\absolute\path\engram.exe"
$env:ENGRAM_RUNTIME_ARGS_JSON = '["--data","C:\absolute\path\.engram"]'
```

设置 `ENGRAM_ID` 后，会启用 `before_agent_start` 自动守护。扩展会把结构化归属保存在消息详情中，并把带归属的文本封套显示为注入消息。

Pi 守护交给 Engram 的是当前真实分支的一段有序对话，而不再只是最新提示：

- 第一次唤醒会提交可见分支末尾和当前提示。Pi 在保存当前提示前触发 `before_agent_start`，所以接入层会明确把它补在切片末尾。
- 一轮成功结束后，接入层会在 Pi 分支内保存一条不进入模型上下文的游标。以后只提交游标之后的新消息；上一轮则已经成为 Engram 自己 Journal 中的亲历历史。
- 可见的用户文字、主 Agent 文字、工具调用和工具结果保留原有顺序与角色。工具调用编号也会保留，使并行调用能够与各自结果对应。当前提示的图片附件会留下带类型的占位说明；隐藏思考、图片二进制、无法表达的消息种类和先前的 `engram-accompaniment` 注入都会被排除，并明确计数。
- 切片采用 64 KiB UTF-8 上限。若无法容纳，会标明省略了多少前缀及是否发生字节截断，不能把残缺切片冒充完整现场。
- 主 Agent 消息、工具结果，以及同一次运行中排队进入的用户 steering（中途引导）或 follow-up（后续消息）都会以外部观察写回 Engram。最初的用户提示已经在唤醒现场中，因此不会重复写入。只有所有可取得的观察都成功写回后，游标才前进；写入失败会让下一次唤醒重放该轮，而不是静默遗失。

这条游标属于 Pi 会话状态，不进入主模型上下文。关闭 Pi 会释放这段有界陪伴；以后重新打开同一 Pi 会话时，可以开始新的陪伴，同时 Engram 仍沿用自己已经积累的历史。

Pi 的主 Agent 可以调用三个 self-fold 工具，但自动 guardian 不会替它调用。观察到工具结果或用户后续消息，并不会暗中把该消息晋升成 Outcome。

## 其他宿主

其余接入保持开放，但 v0 不实现。任何支持 MCP 的宿主都能使用十个显式工具。直接 Hook 接入需要稳定会话 ID、提示词开始事件、当前上下文注入，最好还有回答结束与会话结束事件。通用 Hook 仍只接受 `before_prompt`、`after_response`、`session_end` 三类 JSON 事件，没有自动 Outcome 事件。

## 数据披露

宿主提示词、被观察的回答、被引用的 Outcome 内容、自我折叠与回退都会追加到选定 Engram 的本地 Journal。唤醒时，累计的 v0 上下文会发送给配置的模型提供方；调用 `engram_outcome` 时，还会立即把这份上下文和所引 Outcome 发送给模型作出折叠判断。在敏感工作区开启守护或提交 Outcome 前，用户应理解这条边界。
