---
name: begin-task
description: Start working on a GitHub issue — reads the issue, creates a branch, checks it out, and enters planning mode
user-invokable: true
---

# Begin Task

Jump into any GitHub issue non-linearly. Reads the issue, creates and checks out
a branch, then enters planning mode so you can design the approach before writing code.

## Usage

`/begin-task <issue-number-or-url>`

Argument: $ARGUMENTS

- Accepts a GitHub issue number (e.g. `12`) or a full issue URL (e.g. `https://github.com/owner/repo/issues/12`).
- If no argument is provided, ask the user which issue they want to work on.

## Instructions

### Step 1 — Parse the argument

1. If the argument is a full URL, extract the issue number from it.
2. If the argument is just a number, use it directly.
3. If no argument was provided, run `gh issue list --state open --limit 20` to show
   available issues and ask the user which one to start.

### Step 2 — Read the issue

1. Run `gh issue view <issue-number>` to fetch the issue title, body, labels, and comments.
2. Save the issue number and title for later steps.

### Step 3 — Ensure clean working tree

1. Run `git status --porcelain` to check for uncommitted changes.
2. If there are uncommitted changes, warn the user and ask how they want to proceed
   (stash, commit, or abort). Do NOT silently discard work.

### Step 4 — Create and checkout the branch

1. Run `git checkout master && git pull origin master` to start from latest master.
2. Derive the branch name using the project convention: `{issue#}-{ticket-id-lowercase-hyphenated}`
   - e.g. issue #11 titled "T3.1 Deposit Workflow" → `11-t31-deposit-workflow`
   - Strip dots from ticket IDs (T3.1 → t31), lowercase, hyphenate the description.
3. Check if the branch already exists locally or remotely:
   - `git branch --list <branch-name>` (local)
   - `git ls-remote --heads origin <branch-name>` (remote)
4. If the branch already exists locally, check it out: `git checkout <branch-name>`
5. If it exists only on remote, fetch and track it: `git checkout -b <branch-name> origin/<branch-name>`
6. If it does not exist at all, create it: `git checkout -b <branch-name>`

### Step 5 — Present the issue and enter planning mode

1. Display a clear summary of the issue to the user:
   - Issue number and title
   - Full issue body / description
   - Any labels or milestones
   - The branch that was checked out
2. Enter planning mode so the user can design the approach before writing code.
   Use the EnterPlanMode tool to switch into planning mode.
