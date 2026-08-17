# Engram development charter

[Simplified Chinese](../zh-CN/DESIGN-CHARTER.md)

> Status: current design contract. It judges whether implementation still serves the project purpose. It is neither a human-review gate nor a new governance workflow.

## Purpose

Engram is not primarily about retrieving more of the past. It asks:

> **How can an Agent session that really existed return from its complete historical position, accompany a bounded stretch of present work, and have an effect without taking over the present?**

The minimum runtime relationship is:

```text
complete original Engram context
+ later accompaniment history experienced by that Engram
+ an explicit wake seam
+ a real slice of the active host thread
-> the Engram chooses speech or silence
-> attributed speech enters the host thread
-> the Engram observes subsequent events
-> release or idle timeout returns it to sleep
-> during or after the accompaniment, a caller may cite a concrete outcome to one exact wake
-> the Engram authors change or no_change
-> an active self-fold becomes part of a later wake
```

## What an Engram is

An Engram is a historical Agent session that can run again.

- Its historical body begins with the complete original messages in their original roles, order, and contents.
- Host-thread messages it actually observes, its own later responses, and observable outcomes let that session continue to grow.
- `engram_id` is an address. `name` and `statement` are catalog information used to recognize it.
- A provider may receive a fresh request on every wake or self-fold, but the runtime must resubmit the continuous context. “The provider keeps no remote state” never means “the Engram has no context.”

None of the following is sufficient by itself:

- an identity prompt saying “you are X”;
- a persona card or standpoint summary;
- several similar chunks retrieved from a store;
- a provider-owned remote thread identifier;
- a human-readable Markdown record.

## What a wake must preserve

### 1. Complete self-context

Original messages are loaded as real conversation messages. They are not flattened into a summary, retrieval corpus, or quoted transcript. If complete loading is no longer possible, the runtime must fail explicitly or enter a future, visible paging protocol. It must not silently omit history while claiming completeness.

### 2. A real slice of the active thread

`scene` must not silently mean “the latest user prompt.” It should contain an ordered, attributed slice of the active host conversation and, when relevant, main-Agent replies and tool results.

Hosts expose different amounts of context. A narrow adapter must mark what it omitted. It must not let the Engram infer that one isolated sentence represents the current relationship.

### 3. The Engram chooses silence or speech

The user, main Agent, or a Hook may select a candidate Engram. The final speech decision belongs to the Engram after its own context is loaded. `<silent>` is a valid decision, not a retrieval failure.

### 4. Accompaniment is not one-shot retrieval

A summon opens a numbered accompaniment. Later rounds reuse the same `accompaniment_id`: Engram replies remain its own replies, while new host content remains external observation. Release or idle timeout ends the accompaniment.

### 5. Speech retains attribution

After speech enters a host, users must still be able to tell who spoke, from which historical standpoint, and within which accompaniment. Attribution is not cryptographic authentication, but display, persistence, and reload must not merge Engram speech into user or main-Agent speech.

## Two entry modes

- **MCP summon:** the user or main Agent deliberately names an Engram. The summoner chooses whom to call; the awakened Engram still chooses how to respond and accompanies more than one turn.
- **Hook guardian:** a host automatically wakes a configured Engram at a lifecycle boundary. Automatic wake does not force speech; the Engram may remain silent.

MCP is the minimum portable integration. Hooks are an enhancement when a host exposes lifecycle signals. Both modes address the same Engram rather than constructing separate personas.

## What growth means

Growth is neither replaying every model output as truth nor assigning the user a permanent ground-truth review job.

The project distinguishes:

- **Original history:** events that actually occurred; later summaries cannot overwrite them.
- **External observation:** what the Engram saw in the host thread.
- **Engram speech:** a judgment the Engram made, not an automatic fact.
- **Outcome and correction:** what happened next and how the user or host responded.
- **Self-fold:** the Engram reorganizes its understanding from those experiences, and that organization becomes part of its next context.

The minimum v0 growth mechanism is outcome-driven self-fold. A caller explicitly links allowed later evidence to one exact wake; the Engram then decides `change` or `no_change`, and a change becomes its current posture immediately. This is the Engram's own update, not a proposal waiting for user approval. The user or host can still correct, pause, release, or append a reversion to the parent posture without deleting history.

An outcome is a scoped causal claim, not ground truth. The runtime verifies local event linkage and declared source kind. It does not authenticate the MCP caller, certify a tool, or verify caller-supplied external content. `source_digest` records the SHA-256 of the content bytes received by the runtime; it is not an attestation of reality. The resulting self-fold is explicitly authored by the Engram with `posture/hypothesis` authority and is never rewritten as a user statement or commitment.

Automatic guardians preserve observations but do not autonomously decide which observation is an outcome. Scheduled growth, uncited growth, scalar reward, and generation machinery remain outside v0. The user does not approve every breath or growth event, and enabling accompaniment must not create a standing review job.

## Permanent boundaries

1. Current explicit user intent outranks every historical judgment.
2. Engram speech is advisory by default; an Engram cannot grant itself host tools or action authority.
3. Summaries, indexes, vector-search results, and `statement` cannot replace original messages.
4. Model self-description, main-Agent adoption, and user silence must not be rewritten as user fact.
5. Missing history, truncation, unavailable host context, and provider failure must be visible.
6. Enabling accompaniment must not create an ongoing Human Review debt for the user.
7. An evidence label, external reference, or content digest limits and identifies a record; none of them may be presented as independent proof that the world matches the record.

## What is not the minimum kernel

Action-authorization ledgers, approval queues, elaborate refusal effects, synthetic-user evaluation, large generation-governance systems, standalone chat clients, and untrusted-code sandboxes may be valuable projects. They cannot substitute for a real historical Agent session returning to the present. They re-enter the main line only when a concrete lived failure requires them.

## Current implementation verdict

| Core requirement | v0.3.0 status |
| --- | --- |
| Load original messages completely in role and order | Implemented |
| Lossless import of provider-native images and structured tool calls | Not implemented; v0 is an explicit text-message import |
| Do not manufacture identity from `statement` or a role prompt | Implemented |
| Named summon and bounded multi-round accompaniment | Implemented |
| Preserve speech attribution through all four host adapters | Implemented |
| MCP caller supplies a real active-thread slice | Interface supports it; caller owns slice quality |
| Pi guardian reads a multi-message slice from the active branch | Implemented and verified against a persisted Pi task |
| Claude Code or Cursor Hook reads an existing host transcript | Their current Hook payloads can include `transcript_path`, but this adapter does not consume it yet; the path can also be absent |
| Codex Hook reads an existing full host thread | Not implemented; a supported full-transcript Hook source has not been confirmed |
| Exact wake/observation IDs and outcome-cited Engram self-fold | Implemented for explicit MCP and Pi tool flows |
| `change`, `no_change`, failed-provider retry, status, and append-only revert | Implemented |
| Automatic guardian promotes observations into outcomes | Not implemented; an explicit `engram_outcome` call is required |
| A real self-fold measurably improves a later real judgment | Not yet proven |

## Development order

```text
preserve continuity after release, restart, and later resummon
-> make original-context capture honest and loss-aware for each supported host
-> consume host-supplied Claude Code and Cursor transcripts with path, size, and omission checks
-> confirm a supported Codex transcript source instead of guessing from local files
-> keep one real outcome -> self-fold -> later-wake trace
-> judge whether the later decision actually changed in the intended way
-> expose causal event IDs through guardian Hooks when real use requires it
-> broader guardian and host capabilities
```

Every step is judged by one question: does this means help a historical session return more truthfully, or merely make the project look more elaborate?

## Source discipline

This charter carries two kinds of evidence:

- The May 2026 v1.0/v1.1 design series explicitly required a complete historical body, independent judgment, silence, attributed speech, and self-fold. It classified “identity prompt plus statement” as a lightweight prototype rather than a complete Engram.
- A raw active-thread slice and bounded multi-round accompaniment were later requirements derived from real integration failures. They are current authoritative corrections, not claims about words present in the earliest documents.

The April 26, 2026 source conversation repeatedly referenced by those early documents is not present in reachable repository history. Until recovered, the project will not claim that every original formulation has been reconstructed.
