# SAME Python SDK

Simple Python wrapper for SAME persistent memory. No dependencies beyond the
Python standard library.

## Quick Start

```python
from same import SameVault

vault = SameVault()

# Add a memory
note_id = vault.add("User prefers dark mode", tags=["preference"])

# Search by meaning
results = vault.search("user preferences")
for r in results:
    print(f"{r['title']} (score={r['score']})")

# Read a specific note
note = vault.get(note_id)
print(note["text"])

# Delete a memory
vault.delete(note_id)
```

## Installation

The SDK is a single file with no external dependencies. Copy it into your
project or install with pip:

```bash
pip install -e /path/to/same_sdk
```

## Prerequisites

- Python 3.10+
- The `same` CLI binary on your `$PATH` (pre-installed in SameVault workspace containers)
- An initialized vault (run `same init` or let your container's startup script handle it)

## API Reference

### `SameVault(vault_path="/data/vault", binary="same", timeout=30)`

Create a vault handle. All operations go through this object.

| Parameter    | Default        | Description                          |
|-------------|----------------|--------------------------------------|
| `vault_path` | `"/data/vault"` | Path to the SAME vault directory     |
| `binary`     | `"same"`        | Name or path of the `same` binary    |
| `timeout`    | `30`            | Default subprocess timeout (seconds) |

### `vault.add(text, *, tags=None, source=None, metadata=None, reindex=True) -> str`

Add a memory. Returns the note ID (relative path from vault root).

```python
vault.add("Deploy target is Fly.io", tags=["infra"], source="onboarding")
```

For batch inserts, disable per-note reindexing and reindex once at the end:

```python
for item in many_items:
    vault.add(item, reindex=False)
vault.reindex()
```

### `vault.search(query, *, limit=5, tags=None, domain=None) -> list[dict]`

Semantic search. Returns a list of result dicts.

```python
results = vault.search("authentication approach", limit=10)
# Each result: {id, path, title, text, score, tags, confidence, trust_state, domain, content_type}
```

### `vault.get(memory_id) -> dict`

Read a specific note by its ID (relative path).

```python
note = vault.get("notes/deploy-target-abc123.md")
# Returns: {id, path, title, text, tags, created_at}
```

### `vault.delete(memory_id) -> bool`

Delete a note and reindex.

```python
vault.delete("notes/deploy-target-abc123.md")
```

### `vault.list_recent(limit=20, *, subdir="notes") -> list[dict]`

List most recently modified notes.

```python
recent = vault.list_recent(5)
# Each result: {id, path, title, modified_at}
```

### `vault.status() -> dict`

Get vault status (note count, DB size, provider info).

```python
info = vault.status()
print(f"Notes: {info['vault']['notes']}, DB: {info['vault']['db_size_mb']} MB")
```

### `vault.forget(*, query=None, tags=None, before=None) -> int`

Bulk-suppress memories from search results. Returns count of suppressed notes.

```python
vault.forget(query="old migration notes")
vault.forget(tags=["deprecated"])
vault.forget(query="temp notes", before="2025-01-01")
```

### `vault.ask(question, *, top_k=5, timeout=60) -> str`

Ask a natural-language question answered from vault context. Requires a chat
provider (Ollama or OpenAI) to be configured.

```python
answer = vault.ask("What did we decide about caching?")
```

### `vault.reindex(*, force=False) -> bool`

Manually trigger a vault reindex. Useful after batch `add()` calls with
`reindex=False`.

```python
vault.reindex(force=True)
```

## Error Handling

The SDK returns empty/falsy values instead of raising exceptions:

- `search()` returns `[]` on error
- `get()` returns `{}` if the note doesn't exist
- `delete()` returns `False` on error
- `status()` returns `{}` on error
- `ask()` returns `""` on error
- `forget()` returns `0` on error

Check the `same_sdk` logger for warnings and errors:

```python
import logging
logging.basicConfig(level=logging.DEBUG)
```

## Architecture

SAME stores memories as markdown files on disk with YAML frontmatter for
metadata (tags, source, content type). The `same` CLI indexes these files
using vector embeddings (via Ollama or OpenAI) for semantic search.

This SDK writes notes as files, then shells out to `same reindex` and
`same search` for indexing and retrieval. Each method call is a separate
subprocess -- no shared state, fully thread-safe.
