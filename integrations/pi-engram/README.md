# Pi adapter

[Simplified Chinese](README.zh-CN.md)

Install with the pinned Node 24 and pnpm 11 toolchain, then load `src/v0.ts` as a Pi extension. Set `ENGRAM_COMMAND` to the binary path and `ENGRAM_RUNTIME_ARGS_JSON` to a JSON array of runtime flags. Set `ENGRAM_ID` only when automatic guardian mode is desired.

## Explicit tools

The extension registers seven tools:

- `engram_summon`
- `engram_wake`
- `engram_observe`
- `engram_release`
- `engram_outcome`
- `engram_fold_status`
- `engram_fold_revert`

The explicit outcome path keeps the `wake_event_id` returned by summon or wake, records later host content with `engram_observe`, and keeps its `observation_event_id`. `engram_outcome` then cites those exact events, or supplies a separately identified external observation. The outcome is persisted and the Engram immediately chooses `change` or `no_change`; no human approval queue is created.

A change becomes the Engram's current self-authored posture for later wakes. `no_change` is recorded without replacing the current posture. `engram_fold_revert` restores the parent posture by appending a correction; it does not delete the outcome, fold, or original messages.

## Automatic guardian

With `ENGRAM_ID` set, the guardian wakes before an Agent run with an ordered, bounded slice of Pi's active branch. It displays attributed Engram speech, writes visible host messages back as external observations, and keeps a private branch cursor so later wakes can send only new material.

Automatic observation is not automatic outcome recognition. The current Hook path does not return wake and observation event IDs to Pi, and the extension never calls `engram_outcome` on its own. A tool result, user follow-up, release, or silence therefore remains an observation until an explicit tool flow cites it as an outcome.

The guardian is fail-open for Pi: if wake or observation persistence fails, the main Agent continues. A failed observation prevents the branch cursor from advancing, so the missed material can be replayed on the next wake.

Outcome content and the accumulated Engram context are sent to the configured Engram provider during self-fold. A recorded source reference or SHA-256 content digest does not authenticate the caller or prove that an external claim is true. See the repository's [Engram v0 kernel](../../docs/en/V0-KERNEL.md) and [security boundary](../../SECURITY.md).
