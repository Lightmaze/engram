# 安全边界

Engram v0 是本地连续性工具，不是不可信代码沙箱。

- 每次唤醒都会把原始上下文和后来观察到的宿主内容发送给 Engram 模型提供方。提交 `engram_outcome` 时，运行时还会立即把 Engram 累计上下文和所引 Outcome 内容发送给配置的模型提供方作出自我折叠判断。请保护密钥，并据此选择工作区。
- Journal 可能包含敏感提示词、回答、Outcome、自我折叠姿态和来源位置；`engram_fold_status` 会把 Fold 历史返回调用者。应保护 `ENGRAM_DATA`，不要把它提交进源码仓库。
- MCP 和 Hook 配置会运行本地程序，只应安装在可信的用户或项目配置中。
- MCP 没有密码学意义上的用户身份认证。来源种类检查只能核对本地事件链与记录角色，不能证明消息由谁提交、工具是否可靠，也不能证明调用者填写的外部内容真实。
- `source_digest` 只是记录 Outcome 时对运行时收到的内容字节计算出的 SHA-256。它不是签名或外部证明，不核验 `source_ref`；能够编辑数据目录的人仍可修改 Journal，因此它也不会使 Journal 具备防篡改能力。
- Engram steering 和 self-fold 姿态都是模型生成的建议性内容。Fold 只改变 Engram 以后使用的上下文；v0 不授予它 shell、文件、网络或宿主工具权限。
- 守护 Hook 默认 fail-open：守护故障不会阻止宿主继续运行。

安全问题请通过 GitHub 私有安全通告报告，不要在公开 issue 中附带密钥或私有 Journal 内容。
