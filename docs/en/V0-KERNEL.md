# Engram v0 kernel

[Simplified Chinese](../zh-CN/V0-KERNEL.md)

## Goal

Prove one complete mechanism: a past Agent session can keep its original context, be summoned by name into a different active Agent process, accompany several exchanges, observe what happened, and return to sleep. When a caller later cites a concrete outcome, the Engram can reorganize its own future posture without rewriting that original history or waiting for human approval.

## Identity and persistence

An Engram is a re-runnable Agent session that owns its complete original context. Continuity is carried by exact imported messages and an append-only local Journal; `engram_id` only makes the session addressable.

The model provider does not need to retain a remote thread for the Engram. The runtime resubmits the same complete context on every call. A stateless provider call never means a contextless call, and the Engram is not recreated by an identity role-play prompt.

`name` and the optional `statement` are catalog metadata that help callers recognize and select an Engram. A `statement` may express the historical standpoint associated with the Engram, but real provider calls do not use it as an identity prompt. Identity and continuity come from the original messages and accumulated Journal; a real imported session remains a complete Engram without a `statement`.

The current import contract preserves supplied text-message roles, order, and contents. It is not yet a lossless archive for provider-native image blocks or structured tool-call payloads, and it does not reconstruct hidden provider state. A caller must not label an import complete when context required by that historical session cannot be represented in messages accepted by the configured provider.

The Journal is JSON Lines: one JSON value per line. Locator and guardian files are disposable indexes; the Engram definition and Journal are the durable source. Outcomes, self-folds, and reversions are appended to this Journal; they never edit the imported messages. Before v0.3 writes any event, it appends a format-guard string that current readers recognize but v0.1/v0.2 JournalEvent decoders reject. Downgrading therefore fails visibly instead of waking while silently ignoring growth and correction history.

An operating-system file lock coordinates Journal access across processes that share one data directory. A long-lived MCP server and short-lived Hook processes cannot interleave one runtime transaction. v0 deliberately trades concurrent throughput for consistent locators, indexes, and appended events. The operating system releases the lock after a process crash; liveness does not depend on interpreting a stale lock file.

## Wake mechanism

1. The caller addresses an exact `engram_id`.
2. The runtime opens an accompaniment period and returns `accompaniment_id`.
3. It loads the Engram's original messages as real conversation messages in their original roles and order—not as a summary, retrieval corpus, or single quoted transcript.
4. It reconstructs later Journal experience in order: wake boundaries, observed scenes, Engram replies, external observations, release boundaries, cited outcomes, historical self-folds (including `no_change`), and host reversions.
5. If a self-fold is active, the runtime adds an explicit authorship boundary and then loads that Engram-authored posture as an `assistant` message. It is not a user statement or system truth.
6. It appends the current active-thread slice that the Engram is now observing.
7. The configured model continues from this continuous context and responds to the current scene.
8. Steering or explicit silence returns to the host together with a `wake_event_id`; `active_fold_event_id` identifies the posture used, when one exists.
9. `wake` and `observe` continue the same accompaniment until `release` or idle timeout.

During later accompaniment rounds, the Engram's earlier responses remain `assistant` messages while new host-thread content is explicitly marked as external observation. v0 sends the whole accumulated v0 context on each wake. This makes preservation semantics unambiguous but means oversized histories can exceed a provider context window. Paging is future work, not a hidden v0 behavior.

A single Journal currently has a 128 MiB read limit. The runtime fails explicitly and refuses to wake above that limit; it never silently truncates history and lets an Engram respond without knowing that part of its history is missing.

## Outcome-driven self-fold

`engram_observe` records exact host content and returns `observation_event_id`. An observation is not automatically an outcome. `engram_outcome` is a separate causal handoff: the caller links one exact `wake_event_id` to allowed later evidence, and the runtime appends an `outcome` event before asking the Engram to produce `change` or `no_change`.

The allowed source kinds have deliberately narrow meanings:

- `user_message` cites a later observation from the same accompaniment whose recorded role is `user`. It proves only what that recorded message says; it does not authenticate the MCP caller or turn the message into world truth.
- `tool_result` cites a later observation from the same accompaniment whose recorded role is a tool-result role. It preserves the recorded tool output but does not certify the tool or every conclusion drawn from it.
- `external_observation` carries caller-supplied `source_ref` and exact `content`. The runtime records them but does not fetch, authenticate, or independently verify the external source.

`source_digest` is the SHA-256 digest of the content bytes received when the outcome was recorded. It does not authenticate `source_ref`, prove that an external observation is true, or turn a caller-supplied role label into ground truth. Engram steering, main-Agent self-report, user silence, and an accompaniment release are not accepted as outcome evidence merely because they appear in the Journal.

After recording an outcome, the runtime immediately asks the configured Engram provider for a self-fold. There is no proposal or human-approval queue. A `change` becomes the current posture; `no_change` is durably recorded but leaves the existing posture active. Every fold records Engram authorship, `posture/hypothesis` authority, `user_ratified=false`, and the event IDs on which it depends.

If the provider call fails, the outcome remains durable and the current posture does not change. Retrying the same request with the same `request_id` resumes from that outcome without duplicating the outcome or fold. `engram_fold_revert` can append a host correction that deactivates the current fold and restores its parent without deleting any outcome or history. Later wakes still see the historical fold and the reason it was corrected; only the final active-fold boundary determines which posture is currently in force.

The next wake receives the current fold behind an explicit active-self-fold boundary. The posture is the Engram's own working hypothesis about later judgment; it is not a new original message, a user commitment, or an independently proven fact. This is the only growth mechanism in v0. Scheduled growth, uncited growth, scalar reward, and automatic generation remain outside the kernel.

## Speech attribution

An Engram response must remain distinguishable from the user and the main Agent after it enters a host. The runtime therefore creates an `attribution` object from the stored Engram record and the current accompaniment:

| Field | Meaning |
| --- | --- |
| `engram_id` | Stable address of the Engram that responded. |
| `name` | Human-readable Engram name. |
| `statement` | Optional historical standpoint; omitted when empty. |
| `accompaniment_id` | The bounded accompaniment to which the response belongs. |

The API calls this **attribution**, not a signature, because it is not a cryptographic proof. In the original design, `statement` can be understood as a semantic or historical-standpoint signature. It does not authenticate bytes, and it is never inserted into the provider context to manufacture or simulate identity.

MCP keeps the existing `steering` text and adds structured attribution. `engram_summon` returns it under `wake.attribution`, `engram_wake` returns it as `attribution`, and `engram_guardian_take` returns it under `pending.attribution` when pending speech exists. The additional field is backward compatible for clients that only read `steering`.

For Hooks that inject text, the runtime wraps non-empty active steering in a visible envelope containing one line of JSON-encoded attribution followed by steering. JSON encoding prevents a newline in a user-supplied name or statement from imitating an envelope boundary. A host-specific size limit may shorten the statement preview and steering, but the runtime marks both truncations explicitly and never lets the host silently cut through attribution. The full attribution remains in structured results. The envelope is for the host and its history; it is not sent into the Engram provider context. A silent decision can still carry attribution in a structured wake result, but a Hook does not inject an empty attributed speech.

An integrating host must preserve this source distinction in both the visible conversation and durable history. Engram speech must not be concatenated into a user message or recorded as though the main Agent authored it. Hosts that cannot store structured metadata should retain the attributed text envelope. Only the leading runtime envelope establishes attribution; model-authored steering cannot create a second authoritative envelope by repeating its header. A generic label such as “Engram advisory steering” is not sufficient when it loses the specific Engram identity and accompaniment.

## States

- `active`: the Engram returned steering.
- `silent`: the Engram returned exactly `<silent>` or no meaningful text.
- `sleeping`: the accompaniment was released or exceeded its idle limit.
- provider and storage failures are visible errors, not fabricated steering.

## MCP tools

| Tool | Purpose |
| --- | --- |
| `engram_create` | Import exact original messages. |
| `engram_list` | List named Engrams. |
| `engram_summon` | First wake and open a multi-round accompaniment. |
| `engram_wake` | Continue the same accompaniment. |
| `engram_observe` | Append exact host content and return its observation event ID. |
| `engram_release` | End the accompaniment. |
| `engram_outcome` | Cite later evidence for one wake and immediately run Engram-authored `change` / `no_change` self-fold. |
| `engram_fold_status` | Inspect the active posture and append-only fold/revert history. |
| `engram_fold_revert` | Append a correction that restores the parent posture without deleting history. |
| `engram_guardian_take` | Retrieve pending Hook steering for a cooperative host. |

## Authority

Engram output is advisory text. A self-fold changes only the Engram's own later posture; it does not enlarge its authority. The runtime does not expose host tools to the Engram model and does not grant it permission to act. The active user and host keep final authority.

## Non-goals

Human review gates, user scoring, approval queues, scheduled or uncited growth, scalar reward, generation governance, action ledgers, synthetic evaluation, host-compaction control, and untrusted-code execution are outside v0.
