#!/bin/bash
# workspace/seeds/seed-project.sh — Seeds the vault with a project template.
#
# If TOPIC is set, uses it as the project description.
# Called by seed-vault.sh; expects VAULT_PATH and optionally TOPIC to be set.

set -euo pipefail

VAULT_PATH="${VAULT_PATH:-/data/vault}"
TOPIC="${TOPIC:-}"

mkdir -p "$VAULT_PATH/project"

# --- Determine project name ---
if [ -n "$TOPIC" ]; then
    PROJECT_DISPLAY="$TOPIC"
else
    PROJECT_DISPLAY="New Project"
fi

# --- project/ARCHITECTURE.md ---
if [ ! -f "$VAULT_PATH/project/ARCHITECTURE.md" ]; then
if [ -n "$TOPIC" ]; then
cat > "$VAULT_PATH/project/ARCHITECTURE.md" << INNEREOF
# Architecture: $PROJECT_DISPLAY

## Overview

_Describe the high-level architecture of the system here._

## Components

| Component | Purpose | Status |
|-----------|---------|--------|
| _name_    | _what it does_ | planned |

## Design decisions

### Decision 1: _Title_

- **Context**: _What prompted this decision?_
- **Decision**: _What was decided?_
- **Consequences**: _What are the trade-offs?_

## Data flow

_Describe how data moves through the system._

## Dependencies

_List external services, libraries, or APIs._
INNEREOF
else
cat > "$VAULT_PATH/project/ARCHITECTURE.md" << 'INNEREOF'
# Architecture

## Overview

_Describe the high-level architecture of the system here._

## Components

| Component | Purpose | Status |
|-----------|---------|--------|
| _name_    | _what it does_ | planned |

## Design decisions

### Decision 1: _Title_

- **Context**: _What prompted this decision?_
- **Decision**: _What was decided?_
- **Consequences**: _What are the trade-offs?_

## Data flow

_Describe how data moves through the system._

## Dependencies

_List external services, libraries, or APIs._
INNEREOF
fi
echo "  Created project/ARCHITECTURE.md"
fi

# --- project/TODO.md ---
if [ ! -f "$VAULT_PATH/project/TODO.md" ]; then
if [ -n "$TOPIC" ]; then
cat > "$VAULT_PATH/project/TODO.md" << INNEREOF
# TODO: $PROJECT_DISPLAY

## Up next

- [ ] Define project scope and requirements
- [ ] Set up development environment
- [ ] Create initial project structure

## In progress

_Move tasks here when you start working on them._

## Done

_Move completed tasks here._

## Ideas / Backlog

_Capture ideas that aren't ready for action yet._
INNEREOF
else
cat > "$VAULT_PATH/project/TODO.md" << 'INNEREOF'
# TODO

## Up next

- [ ] Define project scope and requirements
- [ ] Set up development environment
- [ ] Create initial project structure

## In progress

_Move tasks here when you start working on them._

## Done

_Move completed tasks here._

## Ideas / Backlog

_Capture ideas that aren't ready for action yet._
INNEREOF
fi
echo "  Created project/TODO.md"
fi

# --- CLAUDE.md ---
if [ ! -f "$VAULT_PATH/CLAUDE.md" ]; then
if [ -n "$TOPIC" ]; then
cat > "$VAULT_PATH/CLAUDE.md" << INNEREOF
# Project Vault

You are a coding assistant. The user is building the following project:

> $PROJECT_DISPLAY

## Your role

Help the user design, build, and iterate on this project. You have persistent
memory — use it to track architecture decisions, progress, and context across
sessions.

## Guidelines

- Track architecture decisions in \`project/ARCHITECTURE.md\`.
- Keep the task list in \`project/TODO.md\` up to date as work progresses.
- When you make a design choice, document it — future sessions need that context.
- Write code directly in the terminal. The user's workspace persists.
- Be opinionated: suggest good defaults, flag potential issues early.
- When the user returns after a break, review the vault to catch up on context.

## Vault structure

\`\`\`
project/
  ARCHITECTURE.md — System design and decisions
  TODO.md         — Task tracking
\`\`\`

Create additional files and directories as the project grows. Suggested
additions: \`project/DECISIONS.md\` for an ADR log, \`project/NOTES.md\` for
session-level scratchpad.

## Current project

**$PROJECT_DISPLAY**

Start by helping the user define scope, pick a tech stack, and create the
initial project structure.
INNEREOF
else
cat > "$VAULT_PATH/CLAUDE.md" << 'INNEREOF'
# Project Vault

You are a coding assistant. This vault is set up for building a software project
with persistent context tracking.

## Your role

Help the user design, build, and iterate on their project. You have persistent
memory — use it to track architecture decisions, progress, and context across
sessions.

## Guidelines

- Track architecture decisions in `project/ARCHITECTURE.md`.
- Keep the task list in `project/TODO.md` up to date as work progresses.
- When you make a design choice, document it — future sessions need that context.
- Write code directly in the terminal. The user's workspace persists.
- Be opinionated: suggest good defaults, flag potential issues early.
- When the user returns after a break, review the vault to catch up on context.

## Vault structure

```
project/
  ARCHITECTURE.md — System design and decisions
  TODO.md         — Task tracking
```

Create additional files and directories as the project grows. Suggested
additions: `project/DECISIONS.md` for an ADR log, `project/NOTES.md` for
session-level scratchpad.

Ask the user what they'd like to build, then help them define scope, pick
a tech stack, and create the initial project structure.
INNEREOF
fi
echo "  Created CLAUDE.md"
fi
