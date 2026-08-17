# Engram

[Simplified Chinese](README.zh-CN.md)

> Wake a past Agent session as a named participant in the present.

Engram is a local-first MCP server and harness integration that lets a past
Agent session return as a named, bounded participant in a current Agent
process. It resumes the session's original conversation context rather than
reconstructing a persona from vector retrieval or a summary.

The public repository deliberately centers on the Engram v0 kernel:

1. An **Engram** is anchored in its own original Agent-session context. v0 keeps every imported text message exactly and locally; on wake, those messages continue as real conversation messages in their original roles and order rather than being flattened into a summary or quoted corpus. Provider-native images and structured tool calls are an explicit current limitation described below.
2. The user or main Agent can **summon a named Engram** through MCP. It wakes from its own context, responds to the current scene, accompanies several rounds under one `accompaniment_id`, and sleeps after release or idle timeout.
3. An optional **Hook guardian** performs the same wake automatically at supported host turn boundaries.
4. When traceable later evidence becomes available, a caller can link it to one exact wake. The Engram immediately chooses `change` or `no_change`; a change becomes a reversible current posture for later wakes without rewriting the original session or waiting for human approval.

No standing Human Review, scoring workflow, governance framework, scheduled or uncited growth, generation machinery, synthetic evaluation suite, or experiment archive is part of this repository.

## Two modes

- **MCP summon:** you or the main Agent deliberately selects an Engram—like calling a named guardian with a wand. The Engram accompanies a bounded stretch of work; this is not one-shot retrieval.
- **Hook guardian:** a configured Engram wakes automatically before a host turn and contributes advisory context when the host can accept it.

The host remains in charge of its own model, tools, permissions, skills, and context management. An Engram has no tool authority and cannot override the current user.

## Install from source

Requirements: Go 1.22+.

```sh
go build -o bin/engram ./cmd/engram
```

On Windows, name the output `engram.exe`.

## Configure the Engram model

The Engram model is independent from the host model.

```sh
export ENGRAM_DATA="$HOME/.engram"
export ENGRAM_DRIVER="openai-responses"
export ENGRAM_MODEL="YOUR_MODEL_ID"
export OPENAI_API_KEY="YOUR_API_KEY"
```

`deepseek-chat` is also supported. The deterministic `rule` driver is only for offline tests and must be explicitly enabled.

## Create an Engram

```sh
engram create --file examples/engram-v0.json
engram list
```

Creation and listing are local operations and make no model call.

## Connect an existing harness

| Harness | Explicit summon | Automatic guardian | Adapter |
| --- | --- | --- | --- |
| Codex CLI / desktop / IDE | Native MCP | Direct Hook injection | [`integrations/codex`](integrations/codex/) |
| Claude Code | Native MCP | Direct Hook injection | [`integrations/claude-code`](integrations/claude-code/) |
| Cursor | Native MCP | Hook wake + MCP take | [`integrations/cursor`](integrations/cursor/) |
| Pi | Extension tools | `before_agent_start` injection | [`integrations/pi-engram`](integrations/pi-engram/) |

Codex CLI is a primary target, not merely a GUI companion. Codex CLI, desktop, and IDE use the same local MCP server configuration. ChatGPT on the web cannot directly start a local stdio server.

See [Harness integration](docs/en/HARNESS-INTEGRATION.md).

## MCP lifecycle

```text
engram_create / engram_list
             |
      engram_summon -> accompaniment_id + wake_event_id
             |
      +------+------+
      |             |
engram_wake   engram_observe -> observation_event_id
      |             |
      +-------> engram_outcome
                     |
              change / no_change
                     |
        later wake loads current fold
                     |
              engram_release
```

The server exposes ten tools. `engram_fold_status` shows the current self-authored posture and its history; `engram_fold_revert` appends a correction that restores the parent posture. `engram_guardian_take` remains available for hosts whose Hooks cannot inject context directly.

An outcome may arrive while an accompaniment is active or after it has been released. Release itself never triggers self-fold.

## Honest v0 boundary

- Original context, later observations, outcomes, self-folds, and reversions are stored locally in an append-only Journal. A fold never rewrites the imported messages.
- The first v0.3 Journal write adds a downgrade guard. v0.1/v0.2 binaries then fail visibly instead of silently ignoring newer fold and correction events.
- Each wake sends `all imported original messages -> wake boundary -> active-thread slice`, plus later accompaniment exchanges already kept in the Journal. The provider need not retain a remote thread. v0 does not yet page oversized histories.
- A Journal above 128 MiB fails explicitly; v0 never treats a truncated history as complete.
- MCP and Hook processes that share one data directory are serialized by an operating-system file lock on Linux, macOS, and Windows.
- Host compaction is independent. The Engram does not veto or modify it.
- Import is explicit; v0 does not automatically decide which past session should become an Engram.
- v0 preserves the exact text messages supplied at import, but it does not yet losslessly import provider-native images or structured tool-call payloads. Do not call an imported session complete when context required by that session cannot be represented for the chosen provider.
- The only v0 growth entry is a cited, Engram-authored, reversible self-fold. v0 does not perform scheduled or uncited growth, scalar reward, generation, fork, merge, promotion, or replacement.
- `source_digest` records the bytes received for an outcome. It does not authenticate a caller, verify an external reference, or turn a role label into ground truth.
- Automatic guardians preserve observations, but currently do not decide that an observation is an outcome. Use the explicit tool flow when an outcome should trigger self-fold.
- v0 is not a sandbox for untrusted code.

## Documentation

- [Engram v0 kernel](docs/en/V0-KERNEL.md)
- [Engram development charter](docs/en/DESIGN-CHARTER.md)
- [Harness integration](docs/en/HARNESS-INTEGRATION.md)
- [Security boundary](SECURITY.md)
- [Contributing](CONTRIBUTING.md)
- [Public source snapshot](PUBLIC_SOURCE.json)
- [Apache License 2.0](LICENSE)

English and Simplified Chinese are maintained as separate document editions over one language-neutral binary and protocol.
