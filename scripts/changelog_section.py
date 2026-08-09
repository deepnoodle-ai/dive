#!/usr/bin/env python3
"""Extract one version's section from CHANGELOG.md for a GitHub release body.

The changelog is the only place release notes are written, so publishing a
release reads from it rather than restating it. ``make release-notes`` prints a
section to review; ``make release-publish`` pipes the same output into
``gh release create``.

A missing or empty section is an error: an empty release body is worse than a
failed publish, because the tag is already pushed by the time this runs.
"""

from __future__ import annotations

import argparse
from pathlib import Path
import re
import sys
from typing import Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
CHANGELOG = REPO_ROOT / "CHANGELOG.md"

# "## [1.19.0] - 2026-08-09" and "## [Unreleased]".
HEADING_RE = re.compile(r"^## \[(?P<version>[^\]]+)\](?:\s+-\s+(?P<date>\S+))?\s*$")


def normalize(version: str) -> str:
    """Accept ``v1.19.0``, ``1.19.0``, and ``Unreleased`` alike."""
    version = version.strip()
    if version.lower() == "unreleased":
        return "unreleased"
    return version[1:] if version.startswith(("v", "V")) else version


def sections(text: str) -> list[tuple[str, str]]:
    """Return ``(version, body)`` pairs in the order they appear."""
    found: list[tuple[str, list[str]]] = []
    for line in text.splitlines():
        match = HEADING_RE.match(line)
        if match:
            found.append((normalize(match.group("version")), []))
        elif found:
            found[-1][1].append(line)
    return [(version, "\n".join(body).strip()) for version, body in found]


def section(text: str, version: str) -> str:
    wanted = normalize(version)
    available = sections(text)
    for candidate, body in available:
        if candidate == wanted:
            if not body:
                raise SystemExit(
                    f"CHANGELOG.md section for {version} is empty; "
                    "write the notes before publishing the release."
                )
            return body
    known = ", ".join(candidate for candidate, _ in available[:5])
    raise SystemExit(
        f"No CHANGELOG.md section for {version}. Most recent: {known}"
    )


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="release version, for example v1.20.0")
    parser.add_argument(
        "--changelog",
        type=Path,
        default=CHANGELOG,
        help="changelog to read (default: the repo's CHANGELOG.md)",
    )
    args = parser.parse_args(argv)
    print(section(args.changelog.read_text(encoding="utf-8"), args.version))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
