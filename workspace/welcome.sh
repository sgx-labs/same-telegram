#!/bin/bash
# welcome.sh — SameVault welcome and status command.
# Shows vault status, available commands, and provider info.
# Installed to /usr/local/bin/welcome.sh; aliased as 'welcome'.

# --- Colors (standard ANSI, mapped to Tokyo Night by our terminal) ---
C_RESET='\e[0m'
C_BOLD='\e[1m'
C_DIM='\e[2m'
C_BLUE='\e[34m'
C_CYAN='\e[36m'
C_GREEN='\e[32m'
C_YELLOW='\e[33m'
C_MAGENTA='\e[35m'
C_WHITE='\e[97m'
C_GRAY='\e[90m'

VAULT_PATH="${VAULT_PATH:-/data/vault}"

# --- Header ---
echo ""
echo -e "  ${C_BLUE}${C_BOLD}SameVault${C_RESET}  ${C_DIM}Your AI Cloud Terminal${C_RESET}"
echo -e "  ${C_GRAY}──────────────────────────${C_RESET}"
echo ""

# --- Vault status ---
if command -v same >/dev/null 2>&1 && [ -d "$VAULT_PATH" ]; then
    # Try to get memory count from same CLI
    mem_count=$(same list --path "$VAULT_PATH" 2>/dev/null | wc -l 2>/dev/null || echo "0")
    # Subtract header lines if any (same list may output headers)
    if [ "$mem_count" -gt 0 ]; then
        echo -e "  ${C_GREEN}Vault${C_RESET}  ${mem_count} memories stored"
    else
        echo -e "  ${C_GREEN}Vault${C_RESET}  Empty (start a conversation to build memory)"
    fi
else
    echo -e "  ${C_YELLOW}Vault${C_RESET}  Not initialized yet"
fi

# --- Provider info ---
if [ -n "$ANTHROPIC_API_KEY" ]; then
    echo -e "  ${C_CYAN}Key${C_RESET}    Anthropic API connected"
elif [ -n "$OPENROUTER_API_KEY" ]; then
    echo -e "  ${C_CYAN}Key${C_RESET}    OpenRouter connected ${C_DIM}(use /model to switch)${C_RESET}"
else
    echo -e "  ${C_YELLOW}Key${C_RESET}    No API key set ${C_DIM}(see below)${C_RESET}"
fi

echo ""

# --- Commands cheat sheet ---
echo -e "  ${C_WHITE}${C_BOLD}Commands${C_RESET}"
echo -e "  ${C_GRAY}──────────────────────────${C_RESET}"
echo -e "  ${C_CYAN}claude${C_RESET}        Start AI assistant"
echo -e "  ${C_CYAN}same status${C_RESET}   Vault overview"
echo -e "  ${C_CYAN}same list${C_RESET}     Browse memories"
echo -e "  ${C_CYAN}same --help${C_RESET}   All memory commands"
echo -e "  ${C_CYAN}welcome${C_RESET}       Show this screen"
echo ""

# --- API key help (only if no key) ---
if [ -z "$ANTHROPIC_API_KEY" ] && [ -z "$OPENROUTER_API_KEY" ]; then
    echo -e "  ${C_WHITE}${C_BOLD}Setup${C_RESET}"
    echo -e "  ${C_GRAY}──────────────────────────${C_RESET}"
    echo -e "  ${C_MAGENTA}Option 1:${C_RESET} Anthropic"
    echo -e "    export ANTHROPIC_API_KEY=sk-ant-..."
    echo ""
    echo -e "  ${C_MAGENTA}Option 2:${C_RESET} OpenRouter ${C_DIM}(100+ models)${C_RESET}"
    echo -e "    export OPENROUTER_API_KEY=sk-or-..."
    echo -e "    ${C_DIM}Get a key at: openrouter.ai${C_RESET}"
    echo ""
fi
