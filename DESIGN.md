# cerebrai — Design Document

Status: **Draft v0.1** — living document, to be revised as decisions firm up.
Last updated: 2026-09-04

## 1. Vision

cerebrai is a personal automation system and "second brain." Users interact
with it through a conversational LLM interface to describe automations in
plain language; cerebrai's LLM writes the actual code that implements them.
Beyond automations, cerebrai maintains a persistent knowledge/memory store
that the LLM draws on across conversations and automations, acting as an
ongoing personal assistant rather than a one-off scripting tool.

Primary audience: **external customers** (a product, not an internal tool),
starting on **macOS**.

## 2. Core Concepts

- **Conversation** — the user's chat interface to the LLM. This is where
  automations are described, refined, and where the assistant answers
  questions using stored memory.
- **Automation** — a discrete piece of LLM-generated code plus metadata
  (trigger, schedule/event, permissions touched, history of edits). Examples:
  "remind me to water plants every Tuesday," "summarize my inbox every
  morning," "when I get an email from X, log it to my notes."
- **Memory / Second Brain** — a persistent knowledge store (notes, facts,
  history) that the LLM can read from and write to, independent of any
  specific automation. First-class in v1, not deferred.
- **Trigger** — what causes an automation to run: schedule (cron-like),
  external event/webhook, or on-demand conversational request.

## 3. Architecture (initial direction)

```
┌─────────────────────┐
│   Desktop App (UI)   │  Native, full-window macOS app.
│  chat + automation   │  Primary way users interact with cerebrai.
│  management surface  │
└──────────┬───────────┘
           │ IPC / local API
┌──────────▼───────────┐
│   Background Worker   │  Long-running local service.
│ - schedule/trigger     │  Owns automation execution, memory store,
│   evaluation           │  and LLM orchestration. Runs even when
│ - automation execution │  the UI is closed.
│ - memory store         │
│ - LLM orchestration    │
└──────────┬───────────┘
           │
┌──────────▼───────────┐
│   CLI (secondary)      │  Debugging/inspection only in v1
│   `cerebrai ...`       │  (current scaffold). Not the primary
└───────────────────────┘  interface going forward.
```

Key shift from the current repo scaffold: the existing Go CLI
(`cmd/cerebrai`, `internal/cli`) becomes a **debugging tool**, not the
product surface. The product surface is a native desktop app talking to a
background worker/daemon that owns execution, scheduling, and memory.

**Background worker: Go**, reusing the existing scaffold
(`internal/telemetry`, `internal/version`) and built around
[Eino](https://github.com/cloudwego/eino) (CloudWeGo's Go framework for LLM
applications) for the agent loop — orchestration/composition of the
conversation flow, tool calling, and model invocation. This resolves the
"background worker stack" open question from the previous draft.

## 4. Automation Execution Model

- **No sandboxing by default.** Automations frequently need real access to
  local system state and apps (macOS Reminders, Notes, filesystem, etc.), so
  a locked-down sandbox would defeat the purpose. This is an explicit,
  accepted tradeoff — see Risks (§7).
- **Codegen language:** not finalized. Leaning toward whatever integrates
  most natively with the target platform's automation surfaces (e.g.
  AppleScript/JXA/Shell for macOS app integration), with Go or Python as
  acceptable alternatives, potentially per-automation depending on what the
  automation needs to touch. **Open design question** — worth a follow-up
  spike comparing codegen ergonomics and reliability across candidates.
- **Triggers supported (v1):**
  - Scheduled / time-based (cron-like)
  - External events / webhooks
  - Conversational / on-demand ("run this now")
- **Review/approval before running:** not decided. Options to evaluate:
  1. Always require user review of generated code before first run or after
     edits.
  2. Risk-tiered review (auto-run read-only/simple automations, require
     review for anything destructive or touching sensitive data).
  3. No review, rely on logging/observability and iteration.
  Given the top stated risk is **reliability/correctness** (not security),
  leaning toward option 1 or 2 for v1 to build user trust, but this needs a
  decision before implementation.

## 5. LLM Integration

- **Agent loop: Eino.** The conversation/agent loop (prompt orchestration,
  tool calling, chaining reasoning → codegen → tool invocation) is built on
  Eino rather than a hand-rolled orchestration layer. This also gives the
  worker a natural place to hang tracing/callbacks, which pairs well with
  the OpenTelemetry setup already in `internal/telemetry`.
- **Multi-provider / pluggable.** The assistant and codegen layer should sit
  behind a provider abstraction rather than hard-coding one vendor, so
  Anthropic, OpenAI, and local models can all be supported. Eino's
  model-provider abstraction is the expected mechanism for this; confirm
  during implementation that it covers the providers we need without
  leaking provider-specific quirks into automation/memory logic.
- Implication: automation codegen, tool definitions (e.g. "write to macOS
  Reminders"), and memory read/write should be exposed to the agent loop as
  Eino tools/components, keeping them provider-agnostic and testable in
  isolation from the LLM itself.

## 6. Platform & Distribution

- **v1: macOS only.** Enables deep integration with native apps (Reminders,
  Notes, Calendar, etc.) without cross-platform abstraction overhead.
- **Long-term: multi-device.** Vision includes automations coordinating
  across devices (e.g. a task triggered on mobile, executed or reflected on
  desktop). This implies an eventual sync layer between installs.
- **Data storage/sync:** local-first for v1 — each install owns its own
  automations and memory store on-disk. Sync across devices is an explicit
  future phase, not v1 scope. When designed, it should preserve the
  local-first/self-hosted posture (e.g. end-to-end encrypted sync) rather
  than becoming a hosted-SaaS dependency.

## 7. Risks & Open Questions

| Risk / Question | Notes |
|---|---|
| **Reliability/correctness of generated automations** (top concern) | Needs a strategy: testing before activation, structured logging of runs, easy rollback/edit-and-retry, clear failure surfacing to the user. |
| No sandboxing | Accepted tradeoff for capability; mitigate via review flow, execution logging, and scoped permissions per automation rather than process isolation. |
| Codegen language undecided | Spike needed; affects reliability, review-ability, and cross-platform story later. |
| Review/approval flow undecided | Directly affects trust and correctness; should be decided before automation execution is built. |
| Multi-device sync (long-term) | Not v1, but v1 data model (how automations/memory are stored) should not preclude it later. |
| Eino provider/tool coverage | Need to confirm Eino's model-provider abstraction actually covers the providers we want (Anthropic, OpenAI, local) and that its tool-calling model fits automation codegen + memory read/write cleanly. |
| Cost control (LLM usage) | Not the top-stated risk, but worth tracking once usage patterns exist — e.g. capping automation LLM calls per run, provider cost differences. |

## 8. Non-Goals (for now)

- Cross-platform support (Windows/Linux) — deferred past v1.
- Hosted/cloud execution of automations — local-first only for v1.
- Sandboxed/restricted code execution — explicitly not pursuing this given
  the need for real system access.

## 9. Next Steps

1. Decide codegen language(s) for automations (spike/prototype comparison).
2. Decide review/approval model for generated automations before they run.
3. Spike the agent loop in Eino: model provider(s), tool-calling for
   automation codegen + memory read/write, and tracing hookup with the
   existing `internal/telemetry` OTel setup.
4. Define the memory/second-brain data model (what's stored, how retrieved,
   how it interacts with automation context) and how it's exposed to Eino
   as a tool/component.
5. Prototype one end-to-end automation (e.g. macOS Reminders integration) to
   validate the trigger → codegen → execution → logging loop before wider
   scope.

---

*This document should be updated as open questions in §4, §5, and §7 are
resolved, and expanded with concrete API/data-model detail once the
architecture in §3 is validated.*
