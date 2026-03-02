---
name: next-task
description: Close out the current task and write a handoff for the next Claude session to pick up
user-invokable: true
---

# Next Task Handoff

Wrap up the current task and write a handoff to memory so the next Claude session
automatically picks up the next GitHub issue.

## Instructions

### Step 1 — Gather current task context

1. Run `git branch --show-current` to get the current branch name.
2. Run `git log origin/master..HEAD --oneline` to list commits made in this session.
3. From the branch name, extract the issue number (e.g. `4-t11-ledger-domain-model` → issue #4).

### Step 2 — Identify the next issue

1. Run `gh issue list --state open --limit 20` to get open issues.
2. Find the lowest-numbered open issue that is higher than the current issue number.
3. Derive the branch name using the project convention: `{issue#}-{ticket-id-lowercase-hyphenated}`
   - e.g. issue #5 "T1.2 Ledger MySQL schema + migrations" → `5-t12-ledger-mysql-schema`

### Step 3 — Write a brief summary of what was done

In 3-5 bullet points, summarise what was built in this session based on the commits and
files changed. Keep it factual and brief — it's context for the next session, not a report.

### Step 4 — Write the handoff to MEMORY.md

Write the following block to the END of the memory file at:
`C:\Users\darre\.claude\projects\c--dev-exchange-ledger-platform\memory\MEMORY.md`

Use the Edit tool to append this section (do not overwrite the whole file):

```
## Pending Handoff
<!-- Written by /next-task — will be auto-executed and removed on next session start -->

**Completed:** {issue#} - {issue title} (branch: {branch name})
{bullet summary of what was done}

**Next session instructions:** Checkout master, pull latest, create branch `{next branch name}`,
checkout to it, read issue #{next issue #} from GitHub, then present a summary of the
requirements to the user and ask if they are ready to begin.
```

### Step 5 — Confirm to the user

Tell the user:
- What was written to the handoff
- That they can now close this session and start a new one (terminal or VSCode — both work)
- The new session will automatically pick up from where we left off
