#!/bin/bash
# Go Tutor — runs in a SEPARATE Git Bash terminal from the main Claude session
# Reads state written by explain-changes.sh, gathers all context upfront,
# then sends a single self-contained prompt to Claude (no tools needed).
# Interactive mode: you can ask follow-up questions after the explanation.

set -euo pipefail

# Use the same Windows-compatible temp path as the hook script
TEMP_DIR=$(cygpath -m "${TMPDIR:-/tmp}" 2>/dev/null || echo "${TMPDIR:-/tmp}")
STATE_FILE="${TEMP_DIR}/claude-explain-state.json"

if [[ ! -f "$STATE_FILE" ]]; then
    echo ""
    echo "  Error: No state file found at: $STATE_FILE"
    echo "  This script is meant to be launched by the explain-changes.sh hook."
    echo ""
    read -p "  Press Enter to close..."
    exit 1
fi

# All paths in the state file are already in mixed Windows format (C:/foo/bar)
FILE_PATH=$(python -c "import json; d=json.load(open(r'${STATE_FILE}')); print(d['file_path'])")
CLAUDE_DIR=$(python -c "import json; d=json.load(open(r'${STATE_FILE}')); print(d['claude_dir'])")
PROJECT_ROOT=$(python -c "import json; d=json.load(open(r'${STATE_FILE}')); print(d['project_root'])")

cd "$PROJECT_ROOT"

SKILL_PATH="${CLAUDE_DIR}/skills/explain/SKILL.md"

echo ""
echo "  ┌──────────────────────────────────────────────┐"
echo "  │         Go Tutor - Explaining Changes         │"
echo "  │   JS -> Go translation of recent code edits   │"
echo "  │                                               │"
echo "  │   You can ask follow-up questions after the   │"
echo "  │   explanation finishes. Type /exit to quit.   │"
echo "  └──────────────────────────────────────────────┘"
echo ""
echo "  File changed: $FILE_PATH"
echo "  Gathering context..."

# --- Gather all context upfront so Claude needs no tools ---

# 1. Read the skill instructions
SKILL_CONTENT=""
if [[ -f "$SKILL_PATH" ]]; then
    SKILL_CONTENT=$(cat "$SKILL_PATH")
fi

# 2. Get the git diff (try uncommitted first, fall back to last commit)
DIFF=$(git diff HEAD 2>/dev/null || true)
if [[ -z "$DIFF" ]]; then
    DIFF=$(git diff HEAD~1 HEAD 2>/dev/null || true)
    DIFF_SOURCE="last commit (HEAD~1..HEAD)"
else
    DIFF_SOURCE="uncommitted changes (working tree vs HEAD)"
fi

# 3. Read the full changed file
FILE_CONTENT=""
if [[ -f "$FILE_PATH" ]]; then
    FILE_CONTENT=$(cat "$FILE_PATH")
fi

echo "  Context gathered. Generating explanation..."
echo "  ─────────────────────────────────────────────────"
echo ""

# Write prompt to a temp file to avoid shell argument length limits
PROMPT_FILE="${TEMP_DIR}/claude-explain-prompt.md"
cat > "$PROMPT_FILE" << PROMPT_EOF
You are a Go tutor for a JavaScript developer who is learning Go from scratch.

## Your teaching instructions

${SKILL_CONTENT}

## Context: what changed

The file that was just modified is: ${FILE_PATH}
Diff source: ${DIFF_SOURCE}

### Git diff

\`\`\`diff
${DIFF}
\`\`\`

### Full file contents (${FILE_PATH})

\`\`\`go
${FILE_CONTENT}
\`\`\`

## What to do

Follow ALL the steps in the teaching instructions above:
1. High-level summary of what this code does and where it fits in the architecture
2. Line-by-line walkthrough: explain EVERY Go concept using the JS comparisons from the reference
3. Use comparison tables (JS vs Go) wherever helpful
4. Call out gotchas that would trip up a JS developer
5. End with 3-5 key takeaway bullet points (mental model shifts from JS)

Be thorough, conversational, and patient. This is a teaching moment, not a code review.
After your explanation, let the user know they can ask follow-up questions about any Go concept.
PROMPT_EOF

# Pipe the prompt via stdin to avoid argument length limits.
# --print mode outputs the response and exits — no tool calls, no permission prompts.
# --allowedTools "" disables all tools so Claude can't get stuck waiting for approval.
cat "$PROMPT_FILE" | claude -p --allowedTools "" --no-session-persistence

echo ""
echo "  ─────────────────────────────────────────────────"
echo "  Explanation complete. You can ask follow-up"
echo "  questions below. Type /exit to quit."
echo "  ─────────────────────────────────────────────────"
echo ""

# Start interactive session with the tutor persona for follow-ups.
# Pass the skill as a system prompt so Claude retains the teaching style.
claude --system-prompt "You are a Go tutor for a JavaScript developer learning Go. Use the project at ${PROJECT_ROOT} for context. Explain Go concepts with JS comparisons. Be thorough and patient."
