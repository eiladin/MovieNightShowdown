# Movie Night Showdown — Handoff / Handback Protocol

Work on this project may pass between agents (and back) at any time. This protocol
makes that seamless: the user can switch agents, say **"continue"**, and the
incoming agent picks up with no verbal briefing. `docs/STATE.md` is the single
source of truth for progress; this file defines how it is read and updated.

## The "continue" contract

When the user says "continue" (or otherwise resumes), the incoming agent must
**not** ask for context. `CLAUDE.md` is auto-loaded and points here. Everything
needed — layout, conventions, where we left off, what's next — lives in the repo.

## `docs/STATE.md` structure

```
## Current status
Phase: <n> — <name>        Status: not_started | in_progress | blocked | done
Updated: <YYYY-MM-DD> by <agent>        Build: green | red (<reason>)

## Next action
<the one concrete next step an incoming agent should take>

## Phase checklist
- [x] done task
- [ ] pending task

## Handoff log (append-only, newest first)
### <YYYY-MM-DD> — <agent> — handback
- Done: ...
- In-flight: ...
- Next: ...
- Files touched: ...
- Verify: <command -> expected result>
```

## Handback — outgoing agent, before stopping

1. **Leave the tree in a known state.** Commit finished work (Conventional
   Commits), or clearly describe any intentionally uncommitted work in the log.
2. **Ensure it builds**, or record exactly why it does not in `Build:` with the
   failing command and error.
3. **Update `Current status`, `Next action`, and the `Phase checklist`** in
   `STATE.md` to reflect reality at this moment (not aspirations).
4. **Append a dated handback entry** to the log (newest first) with Done /
   In-flight / Next / Files touched / Verify.

## Handoff — incoming agent, on "continue"

1. `CLAUDE.md` is already loaded; do **not** ask the user for context.
2. Read in order: `docs/STATE.md` -> the current phase in `docs/EXECUTION.md` ->
   `docs/PLAN.md` as needed.
3. Run the current phase's smoke check / verification (from `EXECUTION.md`) plus
   `curl -fsS localhost:${PORT:-8080}/healthz` if the server exists.
4. **Reconcile:** if reality does not match what `STATE.md` claims, fix
   `STATE.md` first, then proceed.
5. Execute `Next action`. When done (or stopping), perform Handback above.

## Rules

- The handoff log is **append-only** — never rewrite or delete past entries.
- `STATE.md` reflects the **true** state at handback time, never aspirational.
- **Every phase ends green:** build passes and the phase verification succeeds
  before that phase is marked `done`.
- Use **Conventional Commits**.
- Keep the docs in a neutral, professional voice.
- One `Next action` at a time — it must be a single concrete step, not a list.
