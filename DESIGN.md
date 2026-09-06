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
┌─────────────────────────────────────────────────────────────┐
│  Desktop app — single process (cmd/cerebrai-desktop)         │
│                                                             │
│   UI layer (internal/desktopui)                              │
│     chat + automation management surface; native macOS.      │
│     No automation / memory / LLM logic of its own.           │
│                     │                                       │
│                     ▼   app.Client port (internal/app)      │
│   Engine (in-process)                                        │
│     - schedule / trigger evaluation                          │
│     - automation execution                                   │
│     - memory store                                           │
│     - LLM orchestration (Eino)                               │
│     Stays alive while the window is hidden (quit is a         │
│     deliberate tray action), so schedules keep firing.       │
└─────────────────────────────────────────────────────────────┘

CLI (cmd/cerebrai) — a separate entrypoint. Debugging / inspection only
in v1, not the primary interface.
```

Key shift from the current repo scaffold: the existing Go CLI
(`cmd/cerebrai`, `internal/cli`) becomes a **debugging tool**, not the
product surface. The product surface is the native desktop app, and
everything behind its UI — schedule/trigger evaluation, automation
execution, the memory store, LLM orchestration — runs **in-process** in the
same `cmd/cerebrai-desktop` binary, not in a separate worker or daemon. The
window closing only hides it (quit is an explicit tray action), so the
in-process engine stays alive to service schedules and triggers.

The UI layer talks to that engine only through the `app.Client` port
(`internal/app`), which today is a direct in-process implementation
(`internal/storage`'s SQLite client). Keeping the seam means the engine
*could* be split into its own service later without touching the UI, but
that is explicitly not planned for v1.

**Engine stack: Go**, reusing the existing scaffold (`internal/telemetry`,
`internal/config`) and built around
[Eino](https://github.com/cloudwego/eino) (CloudWeGo's Go framework for LLM
applications) for model invocation and, specifically, the automation writer
agent's tool-calling loop (§5) — not for plain chat, which is a direct
model call rather than an Eino agent. This resolves the "background worker
stack" open question from the previous draft (and the decision is now: no
separate worker at all for v1).

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

**Chat is not an agent.** These are two distinct things, each its own
package with its own provider seam — `internal/chat`
(`chat.ConversationProvider`) for chat, `internal/automationagent`
(`automationagent.ModelProvider`) for the writer (§5.6), so the two can run
on different models — and so chat can carry provider-side conversation
continuity (a resumable session) the writer's stateless single-shot runs
don't need:

- **Chat session** (§5.2) — plain, non-agentic request/reply. One
  `chat.ConversationProvider.Reply` call per user message, no tool-execution
  loop, no Eino `react.Agent` involved. This is *all* ordinary conversation
  is.
- **Automation writer agent** (§5.3) — the actual agent loop
  (`internal/automationagent`, built on Eino's ReAct agent). Genuinely
  agentic: multi-round tool calling to author or edit an automation (§2). It
  is invoked, not embedded in chat — chat hands off to it and waits for its
  result (§5.2).

`automationagent.Loop` / `react.Agent` should only ever mean the automation
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
persisted history            storage.SQLite (app.Client)    chat.ConversationProvider
(app.Message)

[]app.Message ──convert──▶ []*schema.Message
                                        │
                                        ▼
                  .Reply(ctx, priorHandle, history) ──▶ (reply, newHandle)
                                        │                      │
                                        │        persist newHandle on the
                                        │        session (provider_session_id)
                         ┌──────────────┴──────────────┐
                         │ normal reply                │ tool call named
                         ▼                              ▼
                 persist as assistant             hand off to automation
                 message, turn done                writer agent (§5.3);
                                                    persist ITS result as
                                                    the assistant reply
```

- One call per turn: `provider.Reply(ctx, priorHandle, history)`. No loop, no
  `compose.ToolsNodeConfig`, no feeding a tool result back into another call
  within the chat turn itself — that multi-round behavior belongs to the
  automation writer, not chat.
- **Provider-side continuity.** `priorHandle` is the provider's own
  conversation handle from the previous turn (the Claude Code CLI's
  `session_id` in dev builds), persisted on the session row as
  `provider_session_id` and empty until the first reply. `Reply` returns the
  handle to store for next turn — it may change (a provider may fork on
  resume), so `SendMessage` writes it back whenever it differs. Once a handle
  exists the claudecode provider passes it as `--resume` and sends only the
  latest user turn, letting the CLI keep the prior context itself. Providers
  with no such concept return `""` and get the full transcript every turn.
- `create_automation` / `edit_automation` (§5.4) are (for a native
  tool-calling provider) bound **only so the model can signal intent in that
  single turn** — chat itself never executes them. If `Reply` returns a
  normal message, persist it and the turn is done. If it returns a
  `ToolCalls` response naming one of those two, chat code does not run tool
  logic inline; it invokes the automation writer `Loop` (§5.3) with the
  call's arguments as its starting task, runs it to completion, and persists
  whatever it returns (result or status) as this turn's assistant reply. Any
  other tool name coming back would be a bug — chat binds nothing else.
- `chat.Reply` converts persisted `[]app.Message` history into
  `[]*schema.Message` (`toSchemaMessages`). The persisted message model has
  no system-role concept, so that conversion only distinguishes
  `"assistant"` from everything-else-as-`User`.
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

### 5.3 Automation writer agent (`internal/automationagent`)

```
create_automation(description)   internal/automationagent  ModelProvider
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

- `automationagent.Loop` (`internal/automationagent/agent.go`) is a thin
  wrapper around `*react.Agent`; `automationagent.New(ctx, provider)`
  compiles one over an `automationagent.ModelProvider`, `Loop.Respond` runs
  one task to completion including any tool-calling rounds. What changed is
  who calls it: only the chat handoff (§5.2), not a per-message
  conversational wrapper.
- The provider is the process-global automation-writer model
  (`automationagent.Provider()`), not the chat session's per-session model —
  production picks it once at user setup, dev builds hard-code it
  (`devmode.AgentModel`).
- Seed message: built from the tool call's arguments, the same way
  `spawn_agent` seeds a sub-`Loop` with a single task string (§5.4) —
  `create_automation`'s `description`, or `edit_automation`'s existing
  source/metadata plus `requested_change` loaded into the starting context.
- Runs independently of the chat session's own history — the automation
  writer doesn't need or want the full chat transcript, just the
  self-contained task handed to it.

### 5.4 Tool calling inside the automation writer

- `internal/automationagent/tools.go` defines the tools the automation
  writer's `Loop` gets via `defaultTools(provider)`, passed into
  `react.AgentConfig.ToolsConfig`.
- Currently one tool: **`spawn_agent`**. It builds a brand-new `Loop` over
  the *same* provider, drives it to completion on a single self-contained
  task string, and returns the sub-loop's final reply as the tool result —
  lets the automation writer delegate a sub-task instead of solving it
  inline and growing its own history.
- **Recursion:** a spawned `Loop` is built the same way
  (`automationagent.New`), so it gets its own `spawn_agent` tool and can
  spawn further sub-loops. Two bounds apply: nesting is capped at
  `maxSpawnDepth` (the current depth rides on the `context`, and the tool
  errors once it would exceed it), and within any one loop the model's
  tool-calling rounds are capped by `react.AgentConfig.MaxStep`
  (`maxAgentSteps`, set explicitly in `automationagent.New`).
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

### 5.6 Provider abstractions (`internal/chat`, `internal/automationagent`)

- Two provider interfaces in two packages, deliberately **not** unified:
  `chat.ConversationProvider` for chat's single-shot `Reply` (§5.2) and
  `automationagent.ModelProvider` for the automation writer's `Loop` (§5.3).
  They have already diverged: the writer's is Eino's `ToolCallingChatModel`
  (for multi-round tool orchestration), while `chat.ConversationProvider` is
  a narrower `Reply(ctx, priorHandle, history) → (reply, newHandle)` — one
  turn, with a provider-side conversation handle threaded through so the
  session can be resumed (§5.2). A concrete provider can still satisfy both
  (`claudecode.ChatModel` does — `Reply` for chat, `Generate` for the
  writer). Splitting the seams lets chat and the writer run on different
  models (a cheaper conversational model vs. a stronger code-writing one).
  The chat model is per-session (`Session.Model`), with its resume handle in
  `sessions.provider_session_id`; the agent model is process-global (§5.3).
  There is no shared `internal/model` package — each package owns its
  interface plus its own `Unconfigured` / `ErrNotConfigured`.
- Concrete providers today:
  - `chat.Unconfigured` / `automationagent.Unconfigured` — placeholders
    returning that package's `ErrNotConfigured` from every call. The
    fallback for an unrecognized/empty session model ID (chat) or an
    unconfigured process-global agent model.
  - `devmode/claudecode.ChatModel` — shells out to the local `claude` CLI
    (`claude -p`), dev-builds only. On the first turn (`Generate`, or `Reply`
    with an empty handle) it flattens history into a system prompt + single
    transcript prompt; once it has the CLI's `session_id`, `Reply` passes it
    as `--resume` and sends only the latest user message, returning the
    (possibly forked) `session_id` for the store to persist. A `--resume`
    that fails (the CLI no longer has that session on disk) is retried once
    with the full transcript, so a stale handle self-heals. **`WithTools`
    rejects any non-empty tool list
    outright** — the CLI drives its own tool use and can't take an Eino tool
    bind; tools reach it via MCP instead (`WithMCP`, see the dev MCP server
    below).
  - The CLI doesn't take an Eino-style `[]*schema.ToolInfo` bind; its
    external-tool mechanism is **MCP**. `RunOptions.Tools`, `AllowedTools`,
    and `DisallowedTools` are tool-*name* selectors (`--tools`,
    `--allowedTools`, `--disallowedTools`) over tools the CLI already knows
    about — built-ins (Bash, Read, etc.) or `mcp__<server>__<tool>`-qualified
    names from an MCP server registered via `MCPConfigPath`/`MCPConfigs`.
    None of the three hands the CLI a brand-new tool's schema by itself —
    only MCP server config does that.
  - **Decision (built): the app runs its own in-process MCP server, dev
    builds only** — `internal/devmode/devmcp`, serving cerebrai's own tools
    (`create_automation`, `edit_automation`) to the `claude` CLI subprocess.
    Transport is **streamable HTTP on a random 127.0.0.1 port** (via
    `github.com/modelcontextprotocol/go-sdk`); truly in-process, so the tool
    handlers share the app's live automation store. `devmcp.Server`
    exposes `ConfigJSON()` (an inline `--mcp-config` document) and
    `ToolNames()`; `devmode.SetMCPBridge` registers it, and
    `devmode.ChatProvider` hands both to `claudecode.WithMCP`, which sets
    `RunOptions.MCPConfigs` + `AllowedTools` + `StrictMCPConfig: true`
    (ignore the dev's own `.mcp.json`) + `PermissionMode: bypassPermissions`
    (dev-only; cerebrai runs automations unsandboxed anyway, §7). Only the
    **chat** seam gets this wiring — the automation writer
    (`automationagent.Provider`, still plain `devmode.Provider`) must not,
    since its own run is what the tools invoke.
    Because the CLI executes MCP tool calls itself inside `RunPromptCtx`
    rather than surfacing them to the caller, the MCP handler for
    `create_automation`/`edit_automation` is where the automation writer
    actually gets invoked for this provider (§5.2's provider caveat) — the
    handler runs the writer to author source, persists it as a **disabled
    draft** (§5.5, §4), and returns a summary as the MCP tool result, which
    the CLI folds into its own final reply text. **Current shortcut:** the
    handler calls the writer provider's single-shot `Generate`, not an Eino
    `Loop` — for the claudecode provider a single `claude -p` is itself an
    agentic run, and the `Loop` can't wrap claudecode anyway (its `WithTools`
    rejects tools). A real `Loop` here waits on a native tool-calling writer
    provider. This is dev-only scaffolding to exercise the flow without a
    hosted API key (§9); a native tool-calling provider (Anthropic/OpenAI
    hosted) wouldn't need this bridge at all — chat's plain `WithTools` bind
    (§5.2) covers it directly.
- **When adding a new provider, check both call sites** — whether it
  supports `WithTools` for chat's single-shot bind, and separately for the
  automation writer's full tool set — don't assume support for one implies
  the other.
- The dev-only model catalog lives in `internal/devmode`:
  `devmode.DefaultChatModel()` / `devmode.AvailableChatModels()` /
  `devmode.AgentModel()` (all gated on `devmode.Enabled()`) and two
  resolvers over the same `devmode/claudecode` provider —
  `devmode.Provider(modelID)` returns it as a bare Eino
  `ToolCallingChatModel` for the writer, `devmode.ChatProvider(modelID)`
  returns the concrete `*claudecode.ChatModel` (with the MCP bridge attached)
  so chat gets its resumable `Reply`. The same CLI wrapper serves both seams
  in dev, so chat-vs-agent is a distinction the callers make, not this
  catalog. Each provider package wraps it:
  - `chat.DefaultModel()` / `chat.AvailableModels()` /
    `chat.ProviderFor(modelID)` / `chat.DefaultProvider()` — the
    per-session chat catalog, each falling back to `chat.Unconfigured`.
  - `automationagent.Provider()` — the single process-global
    automation-writer provider (§5.3), falling back to
    `automationagent.Unconfigured`.

  Nothing wraps these for the UI: `storage.SQLite` (the `app.Client` impl,
  §3) calls `chat.DefaultModel()` directly, and `desktopui` calls
  `chat.AvailableModels()` directly. `storage.SQLite.SendMessage` pulls in
  `chat` (via `chat.Reply` / `chat.ProviderFor`); `cmd/cerebrai-desktop`
  pulls in `automationagent` to feed the dev MCP server its writer.

### 5.7 Wiring status (read before assuming this is live end-to-end)

- `storage.SQLite.SendMessage` calls `chat.Reply(ctx,
  chat.ProviderFor(session.Model), history, priorHandle)` for plain chat
  (§5.2) — **not** `automationagent.New`/`Loop`, which is reserved for the
  automation writer. It reads the session's `provider_session_id` for
  `priorHandle` and writes back the handle `chat.Reply` returns (when it
  changed), in the same transaction as the message inserts. The reply
  generator is an injectable `Replier` field (tests stub it). In production
  builds the provider is `chat.Unconfigured`, so `SendMessage` errors until a
  hosted provider is wired in.
- `chat.Reply` does **not** yet bind `create_automation`/`edit_automation`
  as intent signals, and there's no `ToolCalls` branch / chat→writer handoff
  in chat code. Not needed for the only provider that exists (claudecode
  reaches those tools over MCP, §5.6); a native tool-calling provider will
  need both added.
- `create_automation` / `edit_automation` exist as **MCP tools only**
  (`internal/devmode/devmcp`), served to the dev claudecode provider. They
  are not Eino tools on the automation writer's `Loop` yet, and
  `automationagent` still only has `spawn_agent`.
- The automation store persists authored `source` (`app.Automation.Source`);
  `storage.SQLite` has `GetAutomation` / `CreateAutomation` /
  `UpdateAutomation` for the MCP handlers (not on `app.Client` — dev-only
  consumer). The schema is still one pre-release migration
  (`0001_initial_schema.sql`).
- The dev-mode in-process MCP server (§5.6) is built and wired:
  `cmd/cerebrai-desktop` starts `devmcp.Start` and calls
  `devmode.SetMCPBridge` when `devmode.Enabled()`.
- No tracing/callbacks hooked up to `internal/telemetry` yet.
- Loop budget: `automationagent.New` sets `react.AgentConfig.MaxStep`
  (`maxAgentSteps`) per loop, and `spawn_agent` nesting is capped at
  `maxSpawnDepth` (§5.4). A finer per-task token/time budget is still TODO.

### 5.8 Testing pattern

- `internal/automationagent/agent_test.go` fakes
  `automationagent.ModelProvider` directly (`echoProvider`,
  `spawningProvider`) instead of hitting a real vendor or the `claude` CLI.
  `spawningProvider` keys its behavior off the *last message's role/content*
  rather than a call counter, so the same fake behaves correctly whether
  it's driving the outer loop or a spawned sub-loop — a pattern worth
  reusing for any new tool test.
- `automationagent.Unconfigured` plus `TestLoopRespondUnconfigured`
  establish the "no provider wired" baseline behavior (`ErrNotConfigured`),
  which matters because sessions can exist with an unrecognized/legacy model
  ID. `internal/chat`'s `TestUnconfigured` does the same for the chat seam.
- Once chat's plain `Generate` path is wired (§5.7), it needs its own test
  double distinct from `spawningProvider` — one that returns a
  `create_automation`/`edit_automation` tool call on the *first* `Generate`
  so the chat-to-writer handoff can be tested without a real provider.

### 5.9 Multi-provider intent (original direction, still holds)

- The assistant/codegen layer sits behind the `chat.ConversationProvider` /
  `automationagent.ModelProvider` seams rather than a
  hard-coded vendor so Anthropic, OpenAI, and local models can all be
  supported — confirm as new providers are added that Eino's abstraction
  keeps covering them without leaking vendor-specific quirks (e.g.
  claudecode's MCP-vs-Eino-tools mismatch above) into chat or
  automation/memory logic. A hosted provider maps its own conversation
  continuity onto `Reply`'s handle (a thread/response ID, or `""` if it
  replays full history each turn).

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
3. Automation writer agent scaffold is in place (`internal/automationagent`,
   §5). **Done:** `storage.SQLite.SendMessage` now calls `chat.Reply` for
   plain chat (§5.2, §5.7); the automation store persists authored source
   (`GetAutomation`/`CreateAutomation`/`UpdateAutomation`);
   `create_automation`/`edit_automation` exist as MCP tools; the dev-mode
   in-process MCP server (`internal/devmode/devmcp`) is built and wired to
   the Claude Code CLI provider (§5.6).
   **Remaining:** the chat-side intent bind + `ToolCalls`/handoff branch for
   native providers (§5.7); `create_automation`/`edit_automation` as the
   writer's own Eino tools and running the writer as a real `Loop` rather
   than single-shot `Generate` (§5.6); memory read/write tools; a finer
   per-task token/time budget (a per-loop `MaxStep` and `spawn_agent` depth
   cap are in place, §5.4); tracing hookup with the existing
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
