# Pi 适配器

[English](README.md)

使用锁定的 Node 24 与 pnpm 11 安装依赖，然后把 `src/v0.ts` 作为 Pi 扩展加载。`ENGRAM_COMMAND` 指向二进制，`ENGRAM_RUNTIME_ARGS_JSON` 是运行参数的 JSON 字符串数组。只有需要自动守护时才设置 `ENGRAM_ID`。

## 显式工具

扩展注册七个工具：

- `engram_summon`
- `engram_wake`
- `engram_observe`
- `engram_release`
- `engram_outcome`
- `engram_fold_status`
- `engram_fold_revert`

显式 Outcome 路径需要保留召唤或再次唤醒返回的 `wake_event_id`，再用 `engram_observe` 保存后来出现的宿主内容并保留其 `observation_event_id`。随后，`engram_outcome` 引用这些具体事件，或者提交另行标明来源的外部观察。Outcome 会先持久保存，Engram 随即自行决定 `change`（改变）或 `no_change`（不改变），不建立人工审批队列。

改变会成为 Engram 以后唤醒所使用的当前自写姿态；`no_change` 会被记录，但不替换现有姿态。`engram_fold_revert` 通过追加纠正事件恢复父姿态，不删除 Outcome、Fold 或原始消息。

## 自动守护

设置 `ENGRAM_ID` 后，guardian 会在主 Agent 开始运行前醒来，读取 Pi 当前活跃分支中一段有顺序、有上限的对话。它显示带归属的 Engram 发言，把可见宿主消息作为外部观察写回，并用不进入模型上下文的分支游标让以后唤醒只提交新增内容。

自动观察不等于自动识别 Outcome。当前 Hook 路径不会把唤醒和观察事件编号返回 Pi，扩展也绝不会自行调用 `engram_outcome`。因此，工具结果、用户后续消息、释放或沉默都仍只是观察；只有显式工具流引用它们时，才会成为触发自我折叠的 Outcome。

Pi 中的 guardian 采用 fail-open：唤醒或观察持久化失败时，主 Agent 仍然继续。观察写入失败会阻止分支游标前进，使遗漏内容能够在下一次唤醒时重放。

自我折叠时，被引用的 Outcome 内容和 Engram 累计上下文会发送给配置的 Engram 模型提供方。记录下来的来源位置和 SHA-256 内容摘要都不认证调用者，也不证明外部主张真实。详见仓库的 [Engram v0 最小内核](../../docs/zh-CN/V0-KERNEL.md)与[安全边界](../../SECURITY.zh-CN.md)。
