# Changelog

## v0.3.0

- Add `wake_event_id` and `observation_event_id`, then expose `engram_outcome`, `engram_fold_status`, and `engram_fold_revert` for a ten-tool MCP surface.
- Let a caller cite a later recorded user message, recorded tool result, or separately identified external observation to one exact wake. Source-kind and local event-link checks limit what the record means; `source_digest` is only the SHA-256 of received content bytes, not authentication or proof of external truth.
- Persist the outcome before immediately asking the Engram for `change` or `no_change`. A change becomes the current Engram-authored `posture/hypothesis` without a human approval queue; `no_change` leaves the prior posture active.
- Keep original messages immutable. Later wakes load the current fold behind an explicit Engram-authorship boundary, and `active_fold_event_id` makes the posture in use visible.
- Preserve an outcome when the fold provider fails and allow idempotent retry with the same `request_id`. Reversion appends a correction that restores the parent posture without deleting outcome or fold history.
- Replay historical folds, `no_change`, and reversion reasons as Engram experience while marking only the final active fold as current. Empty wake responses remain valid silence; an empty fold remains an error.
- Add an append-only Journal format guard so v0.1/v0.2 binaries fail visibly on upgraded data instead of silently ignoring v0.3 growth events. Unknown event kinds also fail closed.
- Raise the default provider response ceiling to 4096 tokens, send that ceiling to DeepSeek-compatible chat endpoints, and align supplied guardian wake timeouts with the 120-second provider limit.
- Expand the Pi adapter to seven explicit tools. Automatic Pi and Hook guardians continue to preserve observations but do not automatically promote an observation into an outcome or trigger self-fold.

## v0.2.0

- Pi guardian wake now carries an ordered slice of the real active branch instead of only the latest prompt.
- The first wake includes visible user and assistant messages, tool calls, and tool results; tool-call identifiers preserve parallel call/result pairing, and later wakes use a durable private cursor to send only the new branch delta.
- Prior Engram injections, hidden thinking, image payloads, and unsupported message kinds are excluded and reported explicitly; the 64 KiB UTF-8 limit marks any omitted prefix or truncation.
- Pi writes assistant messages, tool results, and queued user steering or follow-ups back as external observations, without duplicating the top-level prompt already present in the wake scene.
- Guardian index and observation persistence failures now remain visible to Pi; the main Agent continues, but the cursor does not advance and the missed turn is replayed.
- CI now builds the real Go binary for Pi lifecycle tests, so the cross-runtime suite no longer passes by skipping those tests.
- Added the bilingual development charter that fixes the project purpose: complete Engram context plus a real host-thread slice, bounded accompaniment, attributed speech, and no standing Human Review debt.

## v0.1.3

- Add runtime-owned `attribution` to every Engram wake, including the Engram ID, name, optional historical standpoint, and accompaniment ID; this is speech attribution, not a cryptographic signature.
- Preserve the same attribution through MCP structured results, direct Hook injection, Cursor pending retrieval, and Pi message details.
- Give text-only hosts a visible envelope with single-line JSON attribution so injected Engram speech is not later mistaken for user or main Agent speech.
- Keep `statement` out of provider identity prompts; the attribution envelope exists only for host rendering and history preservation.
- Keep Codex attribution intact within its 5000-byte Hook limit, explicitly mark any shortened statement preview or steering, and upgrade v0.1.2 Cursor pending records when they are read.

## v0.1.2

- Correct Engram wake assembly: original messages are now submitted as real conversation messages in their original roles and order instead of being flattened into an `<original_context>` quotation.
- Append an explicit wake boundary and observed active-thread slice after the original session; multi-round accompaniment keeps Engram responses distinct from external host-thread messages.
- Clarify that a provider call retaining no remote state never means dropping or replacing the Engram context.
- Make `statement` an optional catalog note; real provider calls no longer use it to manufacture Engram identity.

## v0.1.1

- Fail explicitly when a Journal exceeds 128 MiB instead of treating a truncated event stream as complete history.
- Coordinate MCP and Hook processes with operating-system file locks on Linux, macOS, and Windows; runtime transactions for one data directory execute serially.
- Enforce automatic consistency between MCP request schemas and Go JSON request types, including timestamp, host-turn, and retry fields.
- Test both the minimum Go 1.22 line and the current release toolchain in CI, with race detection on the current toolchain.

## v0.1.0

- Exact original Engram context and append-only local Journal.
- Named MCP summon, multi-round wake, observation, and release.
- Codex, Claude Code, Cursor, and Pi integration templates.
- Optional automatic Hook guardian.
- Separate English and Simplified Chinese documentation editions.
