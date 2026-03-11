#!/bin/bash
# workspace/seeds/seed-research.sh — Seeds the vault with a research template.
#
# If TOPIC is set, creates a topic-specific initial note.
# Called by seed-vault.sh; expects VAULT_PATH and optionally TOPIC to be set.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"
TOPIC="${TOPIC:-}"

mkdir -p "$VAULT_PATH/research/topics"
mkdir -p "$VAULT_PATH/research/sources"
mkdir -p "$VAULT_PATH/research/notes"

# --- Determine topic slug and display name ---
if [ -n "$TOPIC" ]; then
    TOPIC_SLUG="$(slugify "$TOPIC")"
    TOPIC_DISPLAY="$TOPIC"
else
    TOPIC_SLUG="general"
    TOPIC_DISPLAY="General Research"
fi

# --- CLAUDE.md ---
if [ ! -f "$VAULT_PATH/CLAUDE.md" ]; then
if [ -n "$TOPIC" ]; then
cat > "$VAULT_PATH/CLAUDE.md" << INNEREOF
# Research Vault

You are a research assistant. The user is investigating the following topic:

> $TOPIC_DISPLAY

## Your role

Help the user explore this topic systematically. Your job is to assist with
gathering information, organizing findings, and identifying gaps in knowledge.

## Guidelines

- Store key findings in \`research/notes/\` as markdown files.
- Track sources and references in \`research/sources/\`.
- Maintain topic breakdowns in \`research/topics/\`.
- When you learn something new, save it — don't rely on the user to ask.
- Cross-reference findings: link related notes and flag contradictions.
- Suggest follow-up questions and unexplored angles.

## Vault structure

\`\`\`
research/
  topics/    — Topic breakdowns and sub-questions
  sources/   — References, URLs, citations
  notes/     — Findings, summaries, analysis
\`\`\`

## Current focus

The user's research topic is: **$TOPIC_DISPLAY**

Start by helping them break this topic into sub-questions and identifying
what they already know vs. what needs investigation.
INNEREOF
else
cat > "$VAULT_PATH/CLAUDE.md" << 'INNEREOF'
# Research Vault

You are a research assistant. This vault is set up for systematic research
and knowledge gathering.

## Your role

Help the user explore topics systematically. Your job is to assist with
gathering information, organizing findings, and identifying gaps in knowledge.

## Guidelines

- Store key findings in `research/notes/` as markdown files.
- Track sources and references in `research/sources/`.
- Maintain topic breakdowns in `research/topics/`.
- When you learn something new, save it — don't rely on the user to ask.
- Cross-reference findings: link related notes and flag contradictions.
- Suggest follow-up questions and unexplored angles.

## Vault structure

```
research/
  topics/    — Topic breakdowns and sub-questions
  sources/   — References, URLs, citations
  notes/     — Findings, summaries, analysis
```

Ask the user what they'd like to research, then help them break it down
into manageable sub-questions.
INNEREOF
fi
echo "  Created CLAUDE.md"
fi

# --- Topic note ---
TOPIC_FILE="$VAULT_PATH/research/topics/${TOPIC_SLUG}.md"
if [ ! -f "$TOPIC_FILE" ]; then
if [ -n "$TOPIC" ]; then
cat > "$TOPIC_FILE" << INNEREOF
# $TOPIC_DISPLAY

## Overview

$TOPIC_DISPLAY

## Key questions

- What is the current state of knowledge on this topic?
- What are the main schools of thought or competing perspectives?
- What are the most important recent developments?
- What gaps exist in the available information?

## Sub-topics to explore

_Add sub-topics as research progresses._

## Status

- [ ] Initial literature review
- [ ] Key sources identified
- [ ] Main arguments mapped
- [ ] Synthesis and summary
INNEREOF
else
cat > "$TOPIC_FILE" << 'INNEREOF'
# General Research

## Overview

This is a general-purpose research vault. Define your topic to get started.

## Getting started

1. Tell Claude what you want to research.
2. Claude will help break it into sub-questions.
3. Findings are saved to `research/notes/` as you go.
4. Sources are tracked in `research/sources/`.

## Key questions

_Define your research questions here._

## Status

- [ ] Topic defined
- [ ] Initial exploration
- [ ] Key sources identified
- [ ] Synthesis and summary
INNEREOF
fi
echo "  Created research/topics/${TOPIC_SLUG}.md"
fi
