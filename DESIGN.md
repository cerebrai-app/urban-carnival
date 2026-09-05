# cerebrai — Design Document

Status: **Draft v0.1** — living document, to be revised as decisions firm up.
Last updated: 2026-09-05

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

- **Conversation** — the user's chat interface to the LLM: a plain
  back-and-forth session, not an agent loop. This is where automations are
  described, refined, and where the assistant answers questions using
  stored memory. When intent to create or edit an automation shows up, chat
  hands off to a dedicated **automation writer agent** rather than
  authoring it inline (DESIGN.md §5).
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
(`internal/telemetry`, `internal/config`) and built around
[Eino](https://github.com/cloudwego/eino) (CloudWeGo's Go framework for LLM
applications) for model invocation and, specifically, the automation writer
agent's tool-calling loop (§5) — not for plain chat, which is a direct
model call rather than an Eino agent. This resolves the "background worker
stack" open question from the previous draft.

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

## 5. Chat Session & Automation Writer Agent

**Chat is not an agent.** These are two distinct things sharing the same
`model.Provider` (§5.6):

- **Chat session** (§5.2) — plain, non-agentic request/reply. One
  `Provider.Generate` call per user message, no tool-execution loop, no
  Eino `react.Agent` involved. This is *all* ordinary conversation is.
- **Automation writer agent** (§5.3) — the actual agent loop
  (`internal/agent`, built on Eino's ReAct agent). Genuinely agentic:
  multi-round tool calling to author or edit an automation (§2). It is
  invoked, not embedded in chat — chat hands off to it and waits for its
  result (§5.2).

`internal/agent.Loop` / `react.Agent` should only ever mean the automation
writer from here on — don't reach for it to generate an ordinary chat
reply.

### 5.1 Why the split

Wrapping every chat turn in a ReAct tool-calling loop made the loop's
purpose ambiguous and added tool-calling overhead (and provider
restrictions, §5.6) to messages that are just conversation. Splitting them
means: chat stays a cheap, predictable session that works with any
provider; the agent loop's tools, system prompt, and recursion behavior can
be designed purely around authoring automations, without also having to be
safe/sensible for open-ended chat.

### 5.2 Chat session

```
persisted history                 workerclient                model.Provider
(workerclient.Message)

[]Message ──toSchemaMessages──▶ []*schema.Message
                                        │
                                        ▼
                        provider.WithTools([create_automation, edit_automation])
                                        │
                                        ▼
                                  .Generate(ctx, history)
                                        │
                         ┌──────────────┴──────────────┐
                         │ normal reply                │ tool call named
                         ▼                              ▼
                 persist as assistant             hand off to automation
                 message, turn done                writer agent (§5.3);
                                                    persist ITS result as
                                                    the assistant reply
```

- One call per turn: `provider.WithTools(...).Generate(ctx, history)`. No
  loop, no `compose.ToolsNodeConfig`, no feeding a tool result back into
  another `Generate` call within the chat turn itself — that multi-round
  behavior belongs to the automation writer, not chat.
- `create_automation` / `edit_automation` (§5.4) are bound **only so the
  model can signal intent in that single turn** — chat itself never
  executes them. If `Generate` returns a normal message, persist it and the
  turn is done. If it returns a `ToolCalls` response naming one of those
  two, chat code does not run tool logic inline; it invokes the automation
  writer `Loop` (§5.3) with the call's arguments as its starting task, runs
  it to completion, and persists whatever it returns (result or status) as
  this turn's assistant reply. Any other tool name coming back would be a
  bug — chat binds nothing else.
- `workerclient.toSchemaMessages` converts persisted `[]Message` history
  into `[]*schema.Message`. It currently only distinguishes `"assistant"`
  vs. everything-else-as-`User` — no `System` role support yet, since the
  persisted message model has no system-role concept.
- **Provider caveat — claudecode is a different control flow.** For a
  provider with native, caller-orchestrated tool calling, the diagram above
  is literal: `Generate` returns either a reply or a `ToolCalls` message,
  and chat code decides what happens next. The dev Claude Code CLI provider
  doesn't work that way — the CLI executes tool calls *itself*, inside the
  one `RunPromptCtx` invocation, and only returns after it's done (see
  §5.6's dev MCP server). So for that provider, invoking the automation
  writer happens **inside the MCP tool handler**, synchronously, before
  `Generate` ever returns to chat code — `Generate` just comes back with
  whatever final natural-language text the CLI wrapped around the result.
  Chat code doesn't get a `ToolCalls` message to branch on in this case;
  the handoff already happened by the time it sees a reply.

### 5.3 Automation writer agent (`internal/agent`)

```
create_automation(description)          internal/agent            model.Provider
edit_automation(id, requested_change)
  (from chat handoff, §5.2)

  task ──▶ Loop.Respond(ctx, [task message])
                    │
                    ▼
          react.Agent.Generate  ──┐
                    │              │ tool call?
                    ▼              │  yes → run tool (§5.4/§5.5),
          Provider.Generate ◀──────┘  feed result back, loop again
                    │
                    ▼
          final artifact (code + metadata) ──▶ back to chat as the reply
```

- `agent.Loop` (`internal/agent/agent.go`) is a thin wrapper around
  `*react.Agent`; `agent.New(ctx, provider)` compiles one, `Loop.Respond`
  runs one task to completion including any tool-calling rounds. This is
  unchanged code — what changed is who calls it: only the chat handoff
  (§5.2), not a per-message conversational wrapper.
- Seed message: built from the tool call's arguments, the same way
  `spawn_agent` seeds a sub-`Loop` with a single task string (§5.4) —
  `create_automation`'s `description`, or `edit_automation`'s existing
  source/metadata plus `requested_change` loaded into the starting context.
- Runs independently of the chat session's own history — the automation
  writer doesn't need or want the full chat transcript, just the
  self-contained task handed to it.

### 5.4 Tool calling inside the automation writer

- `internal/agent/tools.go` defines the tools the automation writer's
  `Loop` gets via `defaultTools(provider)`, passed into
  `react.AgentConfig.ToolsConfig`.
- Currently one tool: **`spawn_agent`**. It builds a brand-new `Loop` over
  the *same* provider, drives it to completion on a single self-contained
  task string, and returns the sub-loop's final reply as the tool result —
  lets the automation writer delegate a sub-task instead of solving it
  inline and growing its own history.
- **Recursion:** a spawned `Loop` is built the same way (`agent.New`), so it
  gets its own `spawn_agent` tool and can spawn further sub-loops. Depth is
  bounded only by how many tool-calling rounds the model makes
  (`react.AgentConfig.MaxStep` per loop) — not configured explicitly today,
  so it's whatever Eino's react package defaults to (§5.7).
- **Adding a tool:** define a typed input struct with `jsonschema` tags,
  wrap the handler with `utils.InferTool(name, description, fn)`, and add it
  to the slice `defaultTools` returns.

### 5.5 Automation-specific tools

- **Two tools, not one**, because create and edit need different inputs:
  - `create_automation(description)` — a natural-language description of
    the desired automation (trigger, what it should do).
  - `edit_automation(automation_id, requested_change)` — a reference to an
    existing automation plus the requested change; the tool implementation
    loads that automation's current source + metadata as starting context
    (`create_automation` has no prior source to load).
- Both need automation-store read/write tools inside the writer's loop
  (list/load existing automations, persist a new or edited one plus its
  metadata) — store doesn't exist yet (§9).
- **Output is a draft, not a live automation.** The writer returns authored
  code + metadata, but §4's review/approval flow (still an open decision)
  sits between that output and actually activating the automation. Don't
  auto-activate — surface the draft for whatever review step §4 settles on.

### 5.6 Provider abstraction (`internal/model`)

- `model.Provider` is a type alias for Eino's `ToolCallingChatModel`
  interface, used by *both* chat's single-shot tool-bound `Generate` (§5.2)
  and the automation writer's `Loop` (§5.3) — one abstraction, two
  different calling patterns on top of it.
- Concrete providers today:
  - `model.Unconfigured` — placeholder returning `model.ErrNotConfigured`
    from every call; fallback for an unrecognized/empty session model ID.
  - `model/claudecode.ChatModel` — shells out to the local `claude` CLI
    (`claude -p`), dev-builds only. Flattens history into a system prompt +
    single transcript prompt. **`WithTools` currently rejects any
    non-empty tool list outright** — that's the code as it stands today,
    not the target design (see the dev MCP server below).
  - The CLI doesn't take an Eino-style `[]*schema.ToolInfo` bind; its
    external-tool mechanism is **MCP**. `RunOptions.Tools`, `AllowedTools`,
    and `DisallowedTools` are tool-*name* selectors (`--tools`,
    `--allowedTools`, `--disallowedTools`) over tools the CLI already knows
    about — built-ins (Bash, Read, etc.) or `mcp__<server>__<tool>`-qualified
    names from an MCP server registered via `MCPConfigPath`/`MCPConfigs`.
    None of the three hands the CLI a brand-new tool's schema by itself —
    only MCP server config does that.
  - **Decision: the worker runs its own in-process MCP server, dev builds
    only,** serving cerebrai's own tools (`create_automation`,
    `edit_automation`, and any others chat or the automation writer expose
    to this provider) to the `claude` CLI subprocess. `ChatModel` points
    `RunOptions.MCPConfigPath`/`MCPConfigs` at it and names the tools
    (`mcp__cerebrai__create_automation`, etc.) in `Tools`/`AllowedTools`.
    Because the CLI executes MCP tool calls itself inside `RunPromptCtx`
    rather than surfacing them to the caller, the MCP handler for
    `create_automation`/`edit_automation` is where the automation writer
    actually gets invoked for this provider (§5.2's provider caveat) — the
    handler runs the writer's `Loop` to completion and returns its result
    as the MCP tool result, which the CLI then folds into its own final
    reply text. This is dev-only scaffolding to exercise the loop without a
    hosted API key (§9); a native tool-calling provider (Anthropic/OpenAI
    hosted) wouldn't need this bridge at all — chat's plain `WithTools` bind
    (§5.2) covers it directly.
- **When adding a new provider, check both call sites** — whether it
  supports `WithTools` for chat's single-shot bind, and separately for the
  automation writer's full tool set — don't assume support for one implies
  the other.
- Session → provider resolution lives in `internal/workerclient/agent.go`:
  `DefaultModel()` / `AvailableModels()` (gated on `config.DevEnabled()`),
  `ProviderFor(modelID)`, `DefaultProvider()`.

### 5.7 Wiring status (read before assuming this is live end-to-end)

- `SQLite.SendMessage` still returns a mocked echo reply. It needs to call
  `provider.Generate` directly for plain chat (§5.2) — **not**
  `workerclient.NewAgentLoop`/`agent.Loop`, which is now reserved for the
  automation writer specifically.
- `create_automation` / `edit_automation` don't exist yet as tools, chat
  doesn't bind them yet, and the automation store they depend on doesn't
  exist yet (§9). Today `internal/agent` only has `spawn_agent`, and nothing
  calls `agent.New` outside of `spawn_agent` and tests.
- The dev-mode in-process MCP server (§5.6) doesn't exist yet, and
  `ChatModel` doesn't wire `MCPConfigPath`/`Tools` to it — until that's
  built, dev-mode chat under the claudecode provider has no way to trigger
  the automation writer conversationally.
- No tracing/callbacks hooked up to `internal/telemetry` yet.
- No explicit `MaxStep`/loop-budget configured for the automation writer —
  a runaway tool-calling loop (e.g. repeated `spawn_agent` recursion) is
  currently unbounded in practice.

### 5.8 Testing pattern

- `internal/agent/agent_test.go` fakes `model.Provider` directly
  (`echoProvider`, `spawningProvider`) instead of hitting a real vendor or
  the `claude` CLI. `spawningProvider` keys its behavior off the *last
  message's role/content* rather than a call counter, so the same fake
  behaves correctly whether it's driving the outer loop or a spawned
  sub-loop — a pattern worth reusing for any new tool test.
- `model.Unconfigured` plus `TestLoopRespondUnconfigured` establish the
  "no provider wired" baseline behavior (`ErrNotConfigured`), which matters
  because sessions can exist with an unrecognized/legacy model ID.
- Once chat's plain `Generate` path is wired (§5.7), it needs its own test
  double distinct from `spawningProvider` — one that returns a
  `create_automation`/`edit_automation` tool call on the *first* `Generate`
  so the chat-to-writer handoff can be tested without a real provider.

### 5.9 Multi-provider intent (original direction, still holds)

- The assistant/codegen layer sits behind `model.Provider` rather than a
  hard-coded vendor so Anthropic, OpenAI, and local models can all be
  supported — confirm as new providers are added that Eino's abstraction
  keeps covering them without leaking vendor-specific quirks (e.g.
  claudecode's MCP-vs-Eino-tools mismatch above) into chat or
  automation/memory logic.

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
3. Automation writer agent scaffold is in place (`internal/agent`, §5) —
   remaining work is: wiring `SQLite.SendMessage` to call `provider.Generate`
   directly for plain chat (replacing the mocked echo reply, §5.2 — **not**
   via `workerclient.NewAgentLoop`, which is reserved for the automation
   writer); defining the automation store (list, load, persist automation
   code + metadata); implementing `create_automation`/`edit_automation` as
   both the chat-side tool bind (§5.2) and the writer's own tools (§5.5);
   building the dev-mode in-process MCP server that serves those tools to
   the Claude Code CLI provider (§5.6), including the `ChatModel` wiring to
   point `RunOptions.MCPConfigPath`/`Tools` at it; memory read/write tools;
   a `MaxStep` loop budget; and tracing hookup with the existing
   `internal/telemetry` OTel setup.
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
