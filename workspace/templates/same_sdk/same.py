"""
SAME Python SDK — lightweight wrapper around the SAME CLI.

Provides a Pythonic interface to SAME (Stateless Agent Memory Engine) for
bot developers who want persistent, searchable memory from their Python code.

No external dependencies beyond the Python standard library. Thread-safe
(each call shells out to the SAME binary via subprocess).

Usage:
    from same import SameVault

    vault = SameVault("/data/vault")
    mem_id = vault.add("User prefers dark mode", tags=["preference"])
    results = vault.search("user preferences")
    vault.delete(mem_id)
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import re
import subprocess
import textwrap
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")


def _strip_ansi(text: str) -> str:
    """Remove ANSI escape codes from CLI output."""
    return _ANSI_RE.sub("", text)


def _slugify(text: str, max_len: int = 60) -> str:
    """Convert free text into a filename-safe slug."""
    slug = text.lower()
    slug = re.sub(r"[^a-z0-9 -]", "", slug)
    slug = re.sub(r"\s+", "-", slug.strip())
    slug = re.sub(r"-+", "-", slug).strip("-")
    return slug[:max_len] if slug else "note"


# ---------------------------------------------------------------------------
# SameVault
# ---------------------------------------------------------------------------


class SameVault:
    """Python interface to a SAME vault.

    Wraps the ``same`` CLI binary. Every method shells out to the CLI via
    :func:`subprocess.run`, so the class is inherently thread-safe (no
    shared mutable state).

    Args:
        vault_path: Absolute path to the vault directory. Defaults to
            ``/data/vault`` (the standard location inside SameVault
            workspace containers).
        binary: Name or path of the ``same`` executable. Defaults to
            ``"same"`` (found on ``$PATH``).
        timeout: Default subprocess timeout in seconds. Individual methods
            accept their own ``timeout`` overrides.

    Example::

        vault = SameVault()
        vault.add("User prefers dark mode", tags=["preference"])
        results = vault.search("user preferences")
        for r in results:
            print(r["title"], r["score"])
    """

    def __init__(
        self,
        vault_path: str = "/data/vault",
        binary: str = "same",
        timeout: int = 30,
    ) -> None:
        self.vault_path = vault_path
        self.binary = binary
        self.timeout = timeout

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _run(
        self,
        args: list[str],
        *,
        timeout: int | None = None,
        check: bool = False,
    ) -> subprocess.CompletedProcess[str]:
        """Execute a ``same`` CLI command and return the result.

        Args:
            args: Arguments to pass after the binary name.
            timeout: Override the default timeout (seconds).
            check: If True, raise on non-zero exit code.
        """
        cmd = [self.binary, "--vault", self.vault_path] + args
        env = os.environ.copy()
        env["VAULT_PATH"] = self.vault_path
        logger.debug("Running: %s", " ".join(cmd))
        return subprocess.run(
            cmd,
            env=env,
            capture_output=True,
            text=True,
            timeout=timeout or self.timeout,
            check=check,
        )

    def _run_json(
        self, args: list[str], *, timeout: int | None = None
    ) -> Any:
        """Run a command that supports ``--json`` and parse the output.

        Returns the parsed JSON on success, or ``None`` on failure.
        """
        result = self._run(args + ["--json"], timeout=timeout)
        if result.returncode != 0:
            logger.warning(
                "same %s failed (rc=%d): %s",
                args[0] if args else "?",
                result.returncode,
                result.stderr.strip(),
            )
            return None
        stdout = result.stdout.strip()
        if not stdout or stdout == "null":
            return None
        try:
            return json.loads(stdout)
        except json.JSONDecodeError:
            logger.warning("Failed to parse JSON from same %s", args[0])
            return None

    def _note_dir(self) -> Path:
        """Return the notes subdirectory inside the vault, creating it if
        needed."""
        p = Path(self.vault_path) / "notes"
        p.mkdir(parents=True, exist_ok=True)
        return p

    @staticmethod
    def _make_frontmatter(
        title: str,
        tags: list[str] | None = None,
        source: str | None = None,
        metadata: dict[str, str] | None = None,
    ) -> str:
        """Build YAML frontmatter for a note."""
        lines = ["---"]
        lines.append(f'title: "{title}"')
        if tags:
            lines.append(f"tags: [{', '.join(tags)}]")
        if source:
            lines.append(f'source: "{source}"')
        if metadata:
            for k, v in metadata.items():
                lines.append(f'{k}: "{v}"')
        lines.append("---")
        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def add(
        self,
        text: str,
        *,
        tags: list[str] | None = None,
        source: str | None = None,
        metadata: dict[str, str] | None = None,
        reindex: bool = True,
        timeout: int | None = None,
    ) -> str:
        """Add a memory to the vault.

        Creates a markdown file in ``<vault>/notes/`` and optionally
        triggers a reindex so the note becomes searchable immediately.

        Args:
            text: The content to remember.
            tags: Optional list of tags for filtering/organization.
            source: Optional provenance label (e.g. ``"telegram-bot"``).
            metadata: Optional dict of extra frontmatter fields.
            reindex: Whether to run ``same reindex`` after writing.
                Set to ``False`` when batch-adding many notes, then call
                :meth:`reindex` once at the end.
            timeout: Override default subprocess timeout (seconds).

        Returns:
            The note ID (relative path from vault root, e.g.
            ``"notes/user-prefers-dark-mode-a1b2c3.md"``).

        Example::

            vault.add("User prefers dark mode", tags=["preference"])
            vault.add("Deploy target is Fly.io", source="onboarding")
        """
        # Build a deterministic-ish filename from content hash.
        content_hash = hashlib.sha256(text.encode()).hexdigest()[:6]
        slug = _slugify(text)
        filename = f"{slug}-{content_hash}.md"
        note_path = self._note_dir() / filename
        rel_path = f"notes/{filename}"

        # Build the first line as a title (truncated).
        title = textwrap.shorten(text, width=80, placeholder="...")

        frontmatter = self._make_frontmatter(
            title=title, tags=tags, source=source, metadata=metadata
        )

        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
        body = f"\n# {title}\n\n{text}\n\n---\n*Added: {timestamp}*\n"

        note_path.write_text(frontmatter + body, encoding="utf-8")
        logger.info("Wrote note: %s", rel_path)

        if reindex:
            self._reindex(timeout=timeout)

        return rel_path

    def search(
        self,
        query: str,
        *,
        limit: int = 5,
        tags: list[str] | None = None,
        domain: str | None = None,
        timeout: int | None = None,
    ) -> list[dict]:
        """Semantic search across the vault.

        Args:
            query: Natural-language search query.
            limit: Maximum number of results (default 5).
            tags: Not directly supported by CLI search; included for API
                consistency. Results will be post-filtered by tag if provided.
            domain: Optional domain filter passed to ``--domain``.
            timeout: Override default subprocess timeout (seconds).

        Returns:
            List of result dicts with keys: ``id``, ``text``, ``score``,
            ``title``, ``tags``, ``path``, ``confidence``, ``trust_state``.
            Returns an empty list on error.

        Example::

            results = vault.search("user preferences", limit=10)
            for r in results:
                print(f"{r['title']} (score={r['score']})")
        """
        args = ["search", query, "--top-k", str(limit)]
        if domain:
            args.extend(["--domain", domain])

        data = self._run_json(args, timeout=timeout)
        if not data or not isinstance(data, list):
            return []

        results = []
        for item in data:
            parsed_tags = []
            raw_tags = item.get("tags", "")
            if isinstance(raw_tags, str) and raw_tags:
                try:
                    parsed_tags = json.loads(raw_tags)
                except (json.JSONDecodeError, TypeError):
                    parsed_tags = [t.strip() for t in raw_tags.split(",") if t.strip()]
            elif isinstance(raw_tags, list):
                parsed_tags = raw_tags

            result = {
                "id": item.get("path", ""),
                "path": item.get("path", ""),
                "title": item.get("title", ""),
                "text": item.get("snippet", ""),
                "score": item.get("score", 0.0),
                "tags": parsed_tags,
                "confidence": item.get("confidence", 0.0),
                "trust_state": item.get("trust_state", "unknown"),
                "domain": item.get("domain", ""),
                "content_type": item.get("content_type", ""),
            }
            results.append(result)

        # Post-filter by tags if requested (CLI doesn't support tag filter).
        if tags:
            tag_set = set(tags)
            results = [
                r for r in results if tag_set.intersection(r.get("tags", []))
            ]

        return results

    def get(self, memory_id: str, *, timeout: int | None = None) -> dict:
        """Get a specific memory by its ID (relative path).

        Reads the note file directly from disk. Falls back to empty dict
        on error.

        Args:
            memory_id: The note's relative path (e.g. ``"notes/foo-abc123.md"``).
            timeout: Unused (reads are direct file I/O), kept for API
                consistency.

        Returns:
            Dict with keys: ``id``, ``path``, ``text``, ``title``, ``tags``,
            ``created_at``. Returns empty dict if the note doesn't exist.

        Example::

            note = vault.get("notes/user-prefers-dark-mode-a1b2c3.md")
            print(note["text"])
        """
        full_path = Path(self.vault_path) / memory_id
        if not full_path.is_file():
            logger.warning("Note not found: %s", memory_id)
            return {}

        try:
            content = full_path.read_text(encoding="utf-8")
        except OSError as e:
            logger.error("Failed to read note %s: %s", memory_id, e)
            return {}

        # Parse frontmatter.
        title = ""
        tags: list[str] = []
        body = content

        if content.startswith("---"):
            parts = content.split("---", 2)
            if len(parts) >= 3:
                fm = parts[1]
                body = parts[2].strip()
                # Simple YAML-ish parsing (no PyYAML dependency).
                for line in fm.strip().splitlines():
                    if line.startswith("title:"):
                        title = line.split(":", 1)[1].strip().strip('"')
                    elif line.startswith("tags:"):
                        raw = line.split(":", 1)[1].strip()
                        # Handle [tag1, tag2] format.
                        raw = raw.strip("[]")
                        tags = [t.strip().strip('"') for t in raw.split(",") if t.strip()]

        # Get file timestamps.
        stat = full_path.stat()
        created_at = datetime.fromtimestamp(
            stat.st_ctime, tz=timezone.utc
        ).isoformat()

        return {
            "id": memory_id,
            "path": memory_id,
            "title": title,
            "text": body,
            "tags": tags,
            "created_at": created_at,
        }

    def delete(self, memory_id: str, *, timeout: int | None = None) -> bool:
        """Delete a memory by its ID (relative path).

        Removes the file from disk and triggers a reindex.

        Args:
            memory_id: The note's relative path (e.g. ``"notes/foo-abc123.md"``).
            timeout: Override default subprocess timeout (seconds).

        Returns:
            ``True`` if the note was deleted, ``False`` if it didn't exist
            or an error occurred.

        Example::

            vault.delete("notes/user-prefers-dark-mode-a1b2c3.md")
        """
        full_path = Path(self.vault_path) / memory_id
        if not full_path.is_file():
            logger.warning("Note not found for deletion: %s", memory_id)
            return False

        try:
            full_path.unlink()
            logger.info("Deleted note: %s", memory_id)
        except OSError as e:
            logger.error("Failed to delete note %s: %s", memory_id, e)
            return False

        self._reindex(timeout=timeout)
        return True

    def list_recent(
        self,
        limit: int = 20,
        *,
        subdir: str = "notes",
        timeout: int | None = None,
    ) -> list[dict]:
        """List the most recently modified notes.

        Scans the vault directory for ``.md`` files and returns them
        sorted by modification time (newest first).

        Args:
            limit: Maximum number of results.
            subdir: Subdirectory to scan (default ``"notes"``). Use ``""``
                or ``"."`` to scan the entire vault.
            timeout: Unused (direct file I/O), kept for API consistency.

        Returns:
            List of dicts with keys: ``id``, ``path``, ``title``,
            ``modified_at``. Returns an empty list on error.

        Example::

            recent = vault.list_recent(5)
            for note in recent:
                print(note["title"], note["modified_at"])
        """
        scan_root = Path(self.vault_path)
        if subdir and subdir != ".":
            scan_root = scan_root / subdir

        if not scan_root.is_dir():
            return []

        md_files: list[tuple[float, Path]] = []
        for p in scan_root.rglob("*.md"):
            # Skip hidden dirs and _PRIVATE.
            parts = p.relative_to(Path(self.vault_path)).parts
            if any(part.startswith(".") or part == "_PRIVATE" for part in parts):
                continue
            try:
                md_files.append((p.stat().st_mtime, p))
            except OSError:
                continue

        md_files.sort(key=lambda x: x[0], reverse=True)

        results = []
        for mtime, p in md_files[:limit]:
            rel = str(p.relative_to(Path(self.vault_path)))
            title = self._extract_title(p)
            modified_at = datetime.fromtimestamp(
                mtime, tz=timezone.utc
            ).isoformat()
            results.append({
                "id": rel,
                "path": rel,
                "title": title,
                "modified_at": modified_at,
            })

        return results

    def status(self, *, timeout: int | None = None) -> dict:
        """Get vault status information.

        Returns a dict with vault statistics: note count, database size,
        embedding provider info, etc.

        Args:
            timeout: Override default subprocess timeout (seconds).

        Returns:
            Dict with keys from ``same status --json`` (``vault``,
            ``embedding``, ``chat``, etc.). Returns empty dict on error.

        Example::

            info = vault.status()
            print(f"Notes: {info['vault']['notes']}")
            print(f"DB size: {info['vault']['db_size_mb']} MB")
        """
        data = self._run_json(["status"], timeout=timeout)
        return data if isinstance(data, dict) else {}

    def forget(
        self,
        *,
        query: str | None = None,
        tags: list[str] | None = None,
        before: str | None = None,
        timeout: int | None = None,
    ) -> int:
        """Bulk-delete (suppress) memories matching criteria.

        Uses ``same feedback <path> down`` to suppress matching notes from
        search results. If ``query`` is given, searches for matching notes
        first. If ``tags`` is given, filters by tag. If ``before`` is given
        (ISO date string), only affects notes modified before that date.

        When both ``query`` and ``tags`` are None, this is a no-op.

        Args:
            query: Search query to find notes to suppress.
            tags: Only suppress notes with at least one of these tags.
            before: ISO date string — only suppress notes modified before
                this date (e.g. ``"2025-01-01"``).
            timeout: Override default subprocess timeout (seconds).

        Returns:
            Number of notes suppressed.

        Example::

            # Suppress all notes about old project
            vault.forget(query="old project migration")

            # Suppress notes with specific tags
            vault.forget(tags=["deprecated", "archived"])
        """
        if query is None and tags is None:
            return 0

        # Find candidates via search.
        candidates = []
        if query:
            candidates = self.search(query, limit=50, timeout=timeout)
        else:
            # Without a query, scan the vault for tag matches.
            all_notes = self.list_recent(limit=500)
            for note_meta in all_notes:
                note = self.get(note_meta["id"])
                if note:
                    candidates.append(note)

        # Filter by tags if provided.
        if tags:
            tag_set = set(tags)
            candidates = [
                c for c in candidates
                if tag_set.intersection(c.get("tags", []))
            ]

        # Filter by date if provided.
        if before:
            try:
                cutoff = datetime.fromisoformat(before).replace(
                    tzinfo=timezone.utc
                )
                filtered = []
                for c in candidates:
                    note_path = Path(self.vault_path) / c.get("path", c.get("id", ""))
                    if note_path.is_file():
                        mtime = datetime.fromtimestamp(
                            note_path.stat().st_mtime, tz=timezone.utc
                        )
                        if mtime < cutoff:
                            filtered.append(c)
                candidates = filtered
            except (ValueError, TypeError):
                logger.warning("Invalid 'before' date: %s", before)

        # Suppress each candidate using `same feedback <path> down`.
        suppressed = 0
        for c in candidates:
            path = c.get("path", c.get("id", ""))
            if not path:
                continue
            result = self._run(
                ["feedback", path, "down"], timeout=timeout
            )
            if result.returncode == 0:
                suppressed += 1
            else:
                logger.warning(
                    "Failed to suppress %s: %s", path, result.stderr.strip()
                )

        return suppressed

    def reindex(self, *, force: bool = False, timeout: int | None = None) -> bool:
        """Trigger a vault reindex.

        Call this after batch-adding notes (when using ``add(..., reindex=False)``).

        Args:
            force: If True, re-embed all files regardless of changes.
            timeout: Override default subprocess timeout (seconds).

        Returns:
            ``True`` if reindex succeeded, ``False`` otherwise.

        Example::

            # Batch-add without reindexing each time.
            for text in many_texts:
                vault.add(text, reindex=False)
            vault.reindex()
        """
        return self._reindex(force=force, timeout=timeout)

    def ask(
        self,
        question: str,
        *,
        top_k: int = 5,
        timeout: int | None = None,
    ) -> str:
        """Ask a natural-language question answered from vault context.

        Uses ``same ask`` which searches the vault for relevant notes,
        then uses an LLM to synthesize an answer. Requires a chat provider
        to be configured (Ollama or OpenAI).

        Args:
            question: The question to ask.
            top_k: Number of notes to use as context (default 5).
            timeout: Override default subprocess timeout (seconds).
                Consider a higher value since LLM inference can be slow.

        Returns:
            The synthesized answer as a string. Returns an empty string
            on error.

        Example::

            answer = vault.ask("What did we decide about authentication?",
                               timeout=60)
            print(answer)
        """
        result = self._run(
            ["ask", question, "--top-k", str(top_k)],
            timeout=timeout or 60,
        )
        if result.returncode != 0:
            logger.warning("same ask failed: %s", result.stderr.strip())
            return ""
        return _strip_ansi(result.stdout).strip()

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _reindex(
        self, *, force: bool = False, timeout: int | None = None
    ) -> bool:
        """Run ``same reindex``."""
        args = ["reindex"]
        if force:
            args.append("--force")
        result = self._run(args, timeout=timeout or 60)
        if result.returncode != 0:
            logger.warning("Reindex failed: %s", result.stderr.strip())
            return False
        return True

    @staticmethod
    def _extract_title(path: Path) -> str:
        """Extract the title from a markdown file's frontmatter or first
        heading."""
        try:
            content = path.read_text(encoding="utf-8")
        except OSError:
            return path.stem

        # Try frontmatter title.
        if content.startswith("---"):
            parts = content.split("---", 2)
            if len(parts) >= 3:
                for line in parts[1].strip().splitlines():
                    if line.startswith("title:"):
                        return line.split(":", 1)[1].strip().strip('"')

        # Fall back to first markdown heading.
        for line in content.splitlines():
            stripped = line.strip()
            if stripped.startswith("# "):
                return stripped[2:].strip()

        return path.stem
