# Engram

[English](README.md)

> 让过去的 Agent 会话以具名参与者的身份回到现在。

Engram 是本地优先的 MCP server 与 harness integration，让过去的 Agent 会话以
具名、有边界的历史参与者身份重新进入当前 Agent 进程。它恢复会话原本的对话上下文，
而不是用向量检索或摘要重新制造一个人格。

当前公开仓库仍围绕 Engram v0 最小内核：

1. **Engram（历史会话体）**扎根于自己的原始 Agent 会话上下文。v0 会在本地精确保留每条已导入的文本消息；唤醒时，这些消息按原有角色与顺序作为真正的会话上下文继续使用，不会被压成摘要或引用材料。模型提供方原生图片与结构化工具调用仍是下文明确列出的现有限制。
2. 用户或主 Agent 可以通过 MCP **召唤指定 Engram**。它从自己的上下文醒来，回应当前情境，在同一个 `accompaniment_id` 下陪伴多个来回，最后因释放或空闲超时重新休眠。
3. 可选的 **Hook 守护模式**会在宿主支持的回合边界自动完成同样的唤醒。
4. 后来出现可追溯证据时，调用者可以把它关联到一次具体唤醒。Engram 随即自行决定 `change`（改变）或 `no_change`（不改变）；变化会成为以后唤醒可用、可回退的当前姿态，不改写原始会话，也不等待人工审批。

本仓库不包含持续 Human Review（人工终审）债务、评分流程、治理框架、定时或无来源成长、代际生成机械、合成评测套件或实验档案。

## 两种模式

- **MCP 召唤：**你或主 Agent 主动指定某个 Engram，像挥动法杖召唤具名守护者。它会陪伴一段有边界的工作，不是检索一次就消失。
- **Hook 守护：**预先配置的 Engram 在宿主回合开始前自动醒来；宿主支持注入时，它把建议性内容加入当前上下文。

宿主仍然掌握自己的模型、工具、权限、skills（可复用能力包）和上下文管理。Engram 没有工具执行权，也不能覆盖当前用户。

## 从源码安装

要求 Go 1.22 或更高版本。

```powershell
go build -o bin/engram.exe ./cmd/engram
```

## 配置 Engram 模型

Engram 模型与宿主模型相互独立。

```powershell
$env:ENGRAM_DATA = "$HOME\.engram"
$env:ENGRAM_DRIVER = "openai-responses"
$env:ENGRAM_MODEL = "YOUR_MODEL_ID"
$env:OPENAI_API_KEY = "YOUR_API_KEY"
```

也支持 `deepseek-chat`。确定性的 `rule` 驱动只用于离线测试，必须显式开启。

## 创建 Engram

```powershell
engram create --file examples/engram-v0.json
engram list
```

创建和列举只在本地发生，不调用模型。

## 接入已有 harness

“harness”是承载主 Agent 的程序，包括它的对话循环、工具和权限；本文也称它为“宿主”。

| 宿主 | 主动召唤 | 自动守护 | 适配器 |
| --- | --- | --- | --- |
| Codex CLI／桌面端／IDE | 原生 MCP | Hook 直接注入 | [`integrations/codex`](integrations/codex/) |
| Claude Code | 原生 MCP | Hook 直接注入 | [`integrations/claude-code`](integrations/claude-code/) |
| Cursor | 原生 MCP | Hook 唤醒，再由 MCP 取回 | [`integrations/cursor`](integrations/cursor/) |
| Pi | 扩展工具 | `before_agent_start` 注入 | [`integrations/pi-engram`](integrations/pi-engram/) |

Codex CLI 是第一等接入目标，不是桌面 GUI 的附属品。Codex CLI、桌面端和 IDE 共用本地 MCP 服务配置；网页 ChatGPT 不能直接启动本地标准输入输出进程。

详见[宿主接入指南](docs/zh-CN/HARNESS-INTEGRATION.md)。

## MCP 生命周期

```text
创建 / 列举 Engram
        |
     召唤 -> 陪伴期编号 + 唤醒事件编号
        |
   +----+----+
   |         |
再次唤醒   观察宿主结果 -> 观察事件编号
   |         |
   +------> 记录 Outcome（后来结果）
                  |
              改变 / 不改变
                  |
          以后唤醒装载当前姿态
                  |
                释放
```

服务器共暴露十个工具。`engram_fold_status` 查看 Engram 当前自写姿态与历史；`engram_fold_revert` 以追加纠正事件的方式恢复父姿态；`engram_guardian_take` 供无法直接注入上下文的 Hook 宿主取回待处理发言。

Outcome 可以在陪伴仍然活跃时到达，也可以在释放以后到达；释放本身绝不会触发 self-fold。

## v0 的诚实边界

- 原始上下文、后续观察、Outcome、自我折叠和回退都保存在本地、只追加的 Journal（事件流水）里；Fold（自我折叠）从不改写导入消息。
- v0.3 首次写入 Journal 时会加入降级护栏；此后 v0.1／v0.2 二进制会明确失败，不会静默忽略新版的折叠与纠正事件。
- 每次唤醒会依次发送“全部已导入原始消息 → 唤醒接缝 → 主线程对话切片”，并带上 Journal 已保存的后来陪伴来回；模型提供方无需保存远程线程，v0 也尚不为超大历史做分页。
- Journal 超过 128 MiB 时会明确失败；v0 不会把截断历史冒充完整历史。
- Linux、macOS 和 Windows 上，共用同一数据目录的 MCP 与 Hook 进程由操作系统文件锁按顺序执行。
- 宿主压缩自己的上下文是另一件事。Engram 不否决、也不改写宿主压缩。
- 导入必须明确发生；v0 不自动判断哪段旧会话应该成为 Engram。
- v0 会精确保留导入时提供的文本消息，但尚不能无损导入模型提供方原生的图片或结构化工具调用载荷。如果一段会话所必需的上下文无法用当前模型可接受的形式表达，就不能把它宣称为完整 Engram。
- v0 唯一的成长入口是有具体来源、由 Engram 自写且可回退的 self-fold（自我折叠）。它不进行定时或无来源成长、标量奖励、代际生成、分叉、合并、晋升或替换。
- `source_digest` 只记录运行时收到的 Outcome 内容字节；它不认证调用者、不核验外部引用，也不会把调用者填写的角色标签变成 ground truth（可靠事实依据）。
- 自动守护会保存观察，但当前不会自行把某条观察判定为 Outcome。需要触发 self-fold 时，应使用返回事件编号的显式工具流。
- v0 不是不可信代码沙箱。

## 文档

- [Engram v0 最小内核](docs/zh-CN/V0-KERNEL.md)
- [Engram开发宪章](docs/zh-CN/DESIGN-CHARTER.md)
- [宿主接入指南](docs/zh-CN/HARNESS-INTEGRATION.md)
- [安全边界](SECURITY.zh-CN.md)
- [参与贡献](CONTRIBUTING.zh-CN.md)
- [公开源码快照](PUBLIC_SOURCE.json)
- [Apache 2.0 许可证](LICENSE)

英文和简体中文是分开的文档版本，共同说明同一个与自然语言无关的程序和协议。
