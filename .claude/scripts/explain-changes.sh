#!/bin/bash
# PostToolUse hook: spawns a Go tutor in a new terminal after .go file edits
# This script runs INSIDE the main Claude session — it must exit fast and not block.

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | python -c "import json,sys; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null)

# Only trigger for .go files
[[ "$FILE_PATH" != *.go ]] && exit 0

# Convert /tmp to Windows-compatible mixed path (C:/Users/.../Temp)
# Git Bash /tmp != Windows /tmp — Python and spawned terminals need real Windows paths
TEMP_DIR=$(cygpath -m "${TMPDIR:-/tmp}" 2>/dev/null || echo "${TMPDIR:-/tmp}")

# Debounce: skip if an explain session launched within the last 60 seconds
LOCK_FILE="${TEMP_DIR}/claude-explain.lock"
NOW=$(date +%s)
if [[ -f "$LOCK_FILE" ]]; then
    LAST_RUN=$(cat "$LOCK_FILE" 2>/dev/null || echo "0")
    if (( NOW - LAST_RUN < 60 )); then
        exit 0
    fi
fi
echo "$NOW" > "$LOCK_FILE"

# Resolve paths — .claude/scripts/ → .claude/ → project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLAUDE_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$CLAUDE_DIR")"

# Convert ALL paths to mixed Windows format (C:/foo/bar) for cross-context compatibility
M_FILE=$(cygpath -m "$FILE_PATH" 2>/dev/null || echo "$FILE_PATH")
M_CLAUDE=$(cygpath -m "$CLAUDE_DIR" 2>/dev/null || echo "$CLAUDE_DIR")
M_PROJECT=$(cygpath -m "$PROJECT_ROOT" 2>/dev/null || echo "$PROJECT_ROOT")

# Write state for the runner script
STATE_FILE="${TEMP_DIR}/claude-explain-state.json"
cat > "$STATE_FILE" << EOF
{
  "file_path": "${M_FILE}",
  "claude_dir": "${M_CLAUDE}",
  "project_root": "${M_PROJECT}",
  "temp_dir": "${TEMP_DIR}"
}
EOF

RUNNER="${CLAUDE_DIR}/scripts/explain-runner.sh"

# Launch in a mintty (Git Bash) window — guarantees bash environment
# Use nohup + disown to fully detach so the hook returns immediately
if command -v mintty &>/dev/null; then
    nohup mintty --title "Go Tutor" --exec bash --login "$RUNNER" </dev/null >/dev/null 2>&1 &
    disown
else
    # Unix fallback
    if command -v gnome-terminal &>/dev/null; then
        nohup gnome-terminal --title "Go Tutor" -- bash "$RUNNER" </dev/null >/dev/null 2>&1 &
        disown
    else
        nohup bash "$RUNNER" > "${TEMP_DIR}/claude-explain-output.md" 2>&1 &
        disown
    fi
fi

exit 0
