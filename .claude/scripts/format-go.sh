#!/bin/bash
# Format Go files after Claude edits them
# This hook runs after Write or Edit tool usage

# Read the tool input from stdin (JSON with file_path)
INPUT=$(cat)

# Extract the file path (using python instead of jq for portability)
FILE_PATH=$(echo "$INPUT" | python -c "import json,sys; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null)

# Only format .go files
if [[ "$FILE_PATH" == *.go ]]; then
    if command -v gofmt &> /dev/null; then
        gofmt -w "$FILE_PATH" 2>/dev/null
    fi

    if command -v goimports &> /dev/null; then
        goimports -w "$FILE_PATH" 2>/dev/null
    fi
fi

exit 0
