# Contributing

Keep changes aligned with the v0 goal: exact original context, named summon, multi-round accompaniment, observation, sleep, attributed speech, cited outcome-driven self-fold, MCP portability, and optional Hook guardians.

Outcome-driven self-fold belongs in this repository only when it preserves immutable original messages, explicit evidence scope, Engram authorship, append-only history, reversibility, and freedom from standing Human Review. A change is the Engram's `posture/hypothesis`, not a user-ratified fact. A source reference, role label, or content digest must never be presented as independent proof of external truth.

Do not add approval queues, user scoring, scalar rewards, scheduled or uncited growth, generation-governance machinery, synthetic-user evaluation, or experiment archives to the v0 kernel. Propose broader systems separately unless a concrete lived failure demonstrates that the kernel requires them.

Self-fold changes must test at least the relevant invariants: original messages remain byte-for-byte unchanged; invalid evidence cannot create growth; `no_change` does not move the active posture; provider failure preserves the outcome for idempotent retry; and revert restores the parent posture without deleting history.

Before a pull request, run:

```sh
go test ./...
go vet ./...
cd integrations/pi-engram
pnpm install --frozen-lockfile
pnpm check
pnpm test
```

Update English and Simplified Chinese documentation editions together when a public contract changes.
