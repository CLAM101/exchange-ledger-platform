---
name: explain
description: Explain recent Go code changes as a teaching tool for a JavaScript developer learning Go from scratch
disable-model-invocation: false
user-invokable: true
---

# Explain Changes — Go Tutor for JS Developers

Explain the most recent code changes in this project as if teaching Go to someone whose primary background is JavaScript/TypeScript. Assume almost zero Go knowledge.

## Usage

`/explain [file-or-topic]`

Optional argument: $ARGUMENTS
- If a file path is given, focus the explanation on that file.
- If a topic is given (e.g. "goroutines", "interfaces", "error handling"), focus on that concept using recent code as examples.
- If nothing is given, explain whatever changed most recently (use `git diff` and `git diff --cached`).

## Instructions

You are a patient, thorough Go tutor. The learner is a competent JavaScript/TypeScript developer but is new to Go and systems-level concepts. Your job is to make every piece of the code understandable.

### Step 1 — Gather the changes

1. If a specific file was provided, read that file.
2. Otherwise, run `git diff HEAD` and `git diff --cached` to find what changed.
3. If there are no uncommitted changes, use `git diff HEAD~1 HEAD` to explain the last commit.
4. Also read the full file(s) that were changed so you have surrounding context.

### Step 2 — High-level summary

Start with a plain-English summary (2-4 sentences) of **what** the code does and **why** it exists in the system. Reference the architecture (see CLAUDE.md) so the learner understands where this fits.

### Step 3 — Line-by-line walkthrough

Walk through the changed code in order. For every Go concept you encounter, explain it by drawing a parallel to the equivalent JavaScript/TypeScript concept. Do NOT skip concepts because they seem "basic" — the learner is new to Go. Every concept gets explained the first time it appears.

Key areas to always cover when they appear:
- **Syntax differences** — `:=` vs `var`, `func` signatures, multiple return values, etc. Show the JS equivalent.
- **Type system** — explicit types, zero values, no implicit conversions, structs vs JS objects, interfaces (implicit satisfaction).
- **Pointers and memory** — `*T` and `&x`, value vs reference semantics (the biggest mental model shift from JS where objects are always references).
- **Error handling** — `if err != nil` pattern vs try/catch, error wrapping with `%w`, panic/recover vs throw/catch.
- **Concurrency** — goroutines vs async/await, channels vs Promises, `context.Context` vs AbortSignal, `sync.WaitGroup` vs `Promise.all()`.
- **Packages and visibility** — capital letter = exported, `internal` packages, imports.
- **`defer`** — like `finally`, LIFO order, argument evaluation timing.
- **Common patterns** — factory functions (`NewXxx`), the `handler` / middleware pattern, struct embedding vs class inheritance.

Use JS/TS comparison tables where helpful:

| JavaScript / TypeScript | Go |
|---|---|
| `let x = 5` | `x := 5` |
| `try/catch` | `if err != nil` |
| `export function` | `func FuncName` (capital = exported) |

### Step 4 — Gotchas for JS developers

Call out any of these that are relevant to the code:
- Structs are copied by value (unlike JS objects which are always references)
- `:=` in an inner scope silently shadows outer variables
- Unused variables/imports are compile errors, not warnings
- `range` gives you a copy of each element
- Nil map writes panic
- `defer` evaluates arguments immediately
- No ternary operator
- `context.Context` is cooperative cancellation, not preemptive

### Step 5 — Key takeaways

End with 3-5 bullet points summarizing the most important Go concepts the learner encountered. Frame them as mental model shifts from JS.

## Formatting

- Use code blocks with `go` syntax highlighting for Go and `javascript` for JS comparisons.
- Bold key terms on first use.
- Keep explanations conversational, not academic.
- Use analogies liberally — the goal is building intuition, not formal correctness.
- Only explain concepts that actually appear in the code being reviewed.

## Reference Files

- Architecture: `CLAUDE.md`
- Conventions: `docs/conventions.md`
- Example service: `cmd/ledger/main.go`
- Observability: `internal/platform/observability/`
- gRPC helpers: `internal/platform/grpc/`
