---
name: commit-push-pr
description: Stage, commit, push, and open a PR in one command
user-invokable: true
---

# Commit, Push, PR

Stage changed files, generate a commit message, push, and open a pull request — all in one invocation.

## Usage

`/commit-push-pr [optional commit message override]`

Argument: $ARGUMENTS

- If a message is provided, use it as the commit message.
- If no argument, generate a message from the diff (see Step 2).

## Instructions

### Step 1 — Assess what changed

1. Run `git status` to see staged, unstaged, and untracked files.
2. Run `git diff` and `git diff --cached` to see the actual changes.
3. Run `git branch --show-current` to get the branch name.
4. Run `git log origin/master..HEAD --oneline` to see existing commits on this branch.

**Important:**
- Do NOT stage files that look like secrets (`.env`, credentials, tokens).
- Do NOT stage binary files or build artifacts (`bin/`, `*.exe`).
- If there are no changes to commit, tell the user and stop.

### Step 2 — Stage and commit

1. Stage relevant files by name (prefer `git add <file> <file>` over `git add -A`).
2. Generate a commit message if one was not provided:
   - Look at the diff to understand the nature of the change.
   - Write a concise message (imperative mood, under 72 chars for subject).
   - If there are multiple logical changes, use a subject line + bullet body.
   - Follow commit style from `git log --oneline -10`.
   - **Do NOT add Co-Authored-By or any AI attribution** (per CLAUDE.md).
3. Show the user the proposed commit message and staged files before committing.
4. Create the commit.

### Step 3 — Push

1. Check if the branch has an upstream: `git rev-parse --abbrev-ref @{upstream} 2>/dev/null`
2. If no upstream, push with: `git push -u origin <branch-name>`
3. If upstream exists, push with: `git push`

### Step 4 — Open PR

1. Check if a PR already exists for this branch: `gh pr view --json number 2>/dev/null`
2. **If a PR already exists:** Tell the user the PR was updated with the new push and show the PR URL. Do NOT create a new PR.
3. **If no PR exists:** Create one:
   - Extract the issue number from the branch name (e.g. `6-t13-ledger-repository` → `#6`).
   - Title: Use the GitHub issue title if available, otherwise derive from commits.
   - Body format:

```
## Summary
<3-5 bullet points describing what this PR does>

## Changes
<list of key files changed and why>

## Test plan
<how to verify — reference make test, specific test files, manual steps>

Closes #<issue-number>
```

4. Create the PR: `gh pr create --title "..." --body "..."`
5. Show the user the PR URL.

### Step 5 — Confirm

Report to the user:
- What was committed (files + message)
- The push result
- The PR URL (new or existing)
