# Security

Engram v0 is a local continuity tool, not an untrusted-code sandbox.

- The Engram model receives the imported original context plus later observed host content on every wake. Submitting `engram_outcome` also sends the accumulated Engram context and cited outcome content to the configured provider immediately for the self-fold decision. Protect provider credentials and choose workspaces accordingly.
- Journal files may contain sensitive prompts, responses, outcomes, self-fold postures, and source references. `engram_fold_status` returns fold history to its caller. Keep `ENGRAM_DATA` private and out of source control.
- MCP and Hook configurations execute a local binary. Install them only in trusted user or project configuration.
- MCP has no cryptographic user authentication. Source-kind checks validate local event linkage and recorded roles; they do not prove who supplied a message, whether a tool is reliable, or whether caller-supplied external content is true.
- `source_digest` is only the SHA-256 of content bytes received when an outcome is recorded. It is not a signature or external attestation, does not verify `source_ref`, and does not make the Journal tamper-evident against someone who can edit the data directory.
- Engram steering and self-fold postures are model-generated advisory content. A fold changes only the Engram's later context; v0 gives it no shell, file, network, or host-tool authority.
- Guardian Hooks fail open by default: a guardian outage does not stop the host.

Report vulnerabilities through GitHub's private security advisory flow. Do not include secrets or private Journal data in a public issue.
