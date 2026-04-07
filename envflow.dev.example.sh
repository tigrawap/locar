#!/bin/bash
# Envflow: dev
# Copy this file to envflow.dev.sh and fill in your values.
# envflow.dev.sh is gitignored — your secrets stay local.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# === Source scripts ===
source "$SCRIPT_DIR/.envflows/source-scripts/claude-set-no-flicker.sh"
source "$SCRIPT_DIR/.envflows/source-scripts/claude-limit-context.sh"

# === Shared (same for everyone) ===
# export EXAMPLE_SHARED_VAR=value

# === User-specific (uncomment and fill in your values) ===
# IMPORTANT: These MUST stay commented out in the example so that
# the validation below catches unconfigured copies immediately.
#export SSH_AUTH_SOCK=$SSH_AUTH_SOCK
#export KUBECONFIG=~/kc/YOUR_KUBECONFIG.yaml    # path to your kubeconfig

# === Merge prompts ===
PROMPT_FILE=$(mktemp /tmp/envflow-prompt.XXXXXX)
trap 'rm -f "$PROMPT_FILE"' EXIT
cat \
  "$SCRIPT_DIR/.envflows/source-prompts/agentic-workflow.md" \
  "$SCRIPT_DIR/.envflows/source-prompts/ainav.md" \
  "$SCRIPT_DIR/envflow.dev.md" \
  > "$PROMPT_FILE"

exec claude --dangerously-skip-permissions \
  --append-system-prompt-file "$PROMPT_FILE" \
  "$@"
