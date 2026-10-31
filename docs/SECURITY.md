# Security and local operation

stef runs on a loopback address and makes no background model calls. This audit
reviewed its HTTP/MCP transport, browser rendering, saved history, filesystem
observer, client setup, installers, dependencies, and release checks.

## Trust boundary

Use stef on a trusted local machine with a trusted project and cooperating agents.
Native local processes can access its loopback API without credentials. Private
workspaces separate workflows; they do not authorize mutually hostile agents or
other local users. Do not expose the port through a proxy, tunnel, or public host.
Capability authentication and separate worker permissions remain future work.

Saved source is untrusted display data. stef never executes it, loads its URLs,
or runs it to simulate your app. User-facing reasons come from the working agent;
passive tracking cannot reconstruct its intent or private reasoning.

## Protections implemented

| Area | Protection |
| --- | --- |
| Network | Loopback listener, exact listener Host port, strict browser Origin, and Fetch Metadata checks. Unrelated localhost sites are rejected. |
| Browser | Restrictive script/connect/object/base policies, no-referrer, no-store for private responses, and validated stream messages. Saved source stays escaped text. |
| Input | Strict JSON types and fields, bounded bodies, finite body-read deadlines, separate 64-slot MCP lanes, 30-minute idle transport expiry, and 64 read-only WebSocket connections. |
| History | Root-confined default directory, regular-file and identity checks, symlink/hard-link refusal, private Unix modes, and a 256 MiB journal cap. |
| Observer | Root-confined reads, sensitive paths and common credential markers excluded independently of Git, bounded source and filesystem queues. |
| Installation | Approved HTTPS download hosts checked before each redirect, archive confinement and size limits, and linked destination refusal. |
| Supply chain | SHA256 release integrity checks, immutable CI action commits, patched Go toolchain, and runtime dependency advisory gates. |

The observer caps its source baseline at 64 MiB, with 10,000 files, directories,
and pending paths. The workspace catalog admits up to 1,024 identities without
evicting existing histories. Aggregate retained event memory still depends on
workspace activity; a separate global byte budget remains future work.

The browser shell contains no workflow data. Its initial navigation can be framed
by the allowed IDE schemes; private API and WebSocket requests still require the
same browser origin. Development uses a guarded local proxy instead of granting
all pages on port 5173 access to the production daemon.

## Privacy and retention

Common credential paths, including .env files, private-key files, and local agent
configuration, are excluded even if Git tracks them. Known key markers suppress
the whole observed revision rather than altering its exact diff. The observer
forgets that baseline before a later clean save or deletion.

These deterministic checks cannot recognize every possible secret. Agent reports,
developer notes, exports, and previously saved history can still contain sensitive
information. Files captured before this change are not retroactively scrubbed.

Clear resets one workspace's visible history. Its earlier plaintext remains in
the journal and may exist in exports or backups. It is not secure erasure.
--no-persist keeps a new daemon's history in memory. A full journal rejects new
writes without overwriting old history; stop the daemon and archive its history,
or start without persistence. Clear does not free a full physical journal.

Unix history files use mode 0600 and the default directory uses 0700. Windows
privacy depends on the project's filesystem ACLs; chmod does not establish a
separate Windows per-user authorization boundary.

## Tokens and interactive previews

Tracking, security checks, names, themes, diff inspection, source comparison, and
supported CSS samples make zero model calls. Switching design does not generate
additional explanations. Compare line uses only the exact saved replacement
block and highlights its changed text locally. CSS samples use supported literal
values in an unchanged captured selector; uncertain scope gets no sample.

All activity retains supported sanitized tool stages and brief public decisions.
It does not collect private thinking, raw commands, prompts, or tool output.
The hub stamps each event's original recording mode/window; switching modes
does not expose previously hidden observations.
Agent-authored explanations still consume tokens in that agent's conversation.
Explain more queues an explicit request for that agent's next check-in. stef does
not launch another model. Disabled stops new recording and reporting. This is
zero extra inference, not a promise that agent communication is token-free.

## Verification limits

Regression tests cover malicious browser origins, malformed events, escaped
source, unsafe style values, linked journals/install targets, credential
exclusion, and resource limits. Race tests and browser tests exercise normal
MCP reporting, history, private/sharing behavior, and interactive controls.
See [Validation](VALIDATION.md) for actual platform results.

The audit found no runtime npm advisories after the fixes. The current Svelte
build plugin still depends on deepmerge 4.3.1, which has a low-severity prototype
poisoning advisory with no published upstream fix. It is build tooling and does
not ship in stef's renderer. Keep build configuration trusted; the full dependency
audit remains documented rather than being hidden by a production-only scan.

These changes are in the source checkpoint and updated local companion. The
published v0.3.0 packages predate this audit; a new portable release is pending.

This hardening reduces verified risks. It is not a guarantee against every breach
or a certification of every IDE, operating system, or agent host.
