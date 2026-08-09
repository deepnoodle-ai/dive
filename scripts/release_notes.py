#!/usr/bin/env python3
"""Assemble the GitHub release body for a version under ``docs/releases/``.

A release note has three parts, and only one of them can be generated:

``## Highlights``
    Prose. What actually changed, who it affects, what they should do. Written
    by a human or an agent; this script only reserves the space and preserves
    what is already there.

``## Pull requests``
    Collected from the commit range between the previous release tag and HEAD.
    The repo squash-merges, so every merged PR is one commit ending in
    ``(#NNN)``.

``## Changelog``
    The version's ``CHANGELOG.md`` section, verbatim. Already curated, so it is
    copied rather than restated.

Re-running is safe: an existing file's Highlights are carried over and only the
mechanical sections are refreshed.
"""

from __future__ import annotations

import argparse
from pathlib import Path
import re
import subprocess
import sys
from typing import Sequence

import changelog_section


REPO_ROOT = Path(__file__).resolve().parents[1]
RELEASE_DIR = REPO_ROOT / "docs" / "releases"

VERSION_RE = re.compile(r"^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$")
# Root release tags only. Sub-module tags carry a directory prefix
# ("a2a/v1.19.0") and would otherwise sort into the same list.
ROOT_TAG_RE = re.compile(r"^v\d+\.\d+\.\d+$")
PR_RE = re.compile(r"^(?P<title>.+?)\s+\(#(?P<number>\d+)\)$")
# The release-prep commit is bookkeeping for the release it belongs to.
PREP_RE = re.compile(r"^Prepare v\d+\.\d+\.\d+ release$")

HIGHLIGHTS_PLACEHOLDER = """<!-- Replace this comment with the release story: what changed, who it
affects, and what they need to do. Two to four short paragraphs. The
mechanical detail lives in the sections below - do not restate it here.
`make release-publish` refuses to publish while this comment remains. -->"""


def git(*args: str) -> str:
    process = subprocess.run(
        ["git", *args], cwd=REPO_ROOT, text=True, capture_output=True, check=False
    )
    if process.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} failed: {process.stderr.strip()}")
    return process.stdout.strip()


def previous_tag(version: str) -> str | None:
    """The most recent root release tag that is not the one being released."""
    tags = [
        tag
        for tag in git("tag", "--sort=-version:refname").splitlines()
        if ROOT_TAG_RE.fullmatch(tag) and tag != version
    ]
    return tags[0] if tags else None


def pull_requests(since: str | None) -> list[tuple[int, str]]:
    revision = f"{since}..HEAD" if since else "HEAD"
    subjects = git("log", "--no-merges", "--pretty=%s", revision).splitlines()
    found: list[tuple[int, str]] = []
    for subject in subjects:
        if PREP_RE.match(subject):
            continue
        match = PR_RE.match(subject)
        if match:
            found.append((int(match.group("number")), match.group("title")))
    return found


def existing_highlights(path: Path) -> str | None:
    """The Highlights body of an already-written file, if it has one."""
    if not path.exists():
        return None
    body: list[str] = []
    capturing = False
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("## "):
            if capturing:
                break
            capturing = line.strip() == "## Highlights"
            continue
        if capturing:
            body.append(line)
    text = "\n".join(body).strip()
    return text or None


def prose(highlights: str) -> str:
    """``highlights`` with HTML comments removed, to tell a draft from a note."""
    return re.sub(r"<!--.*?-->", "", highlights, flags=re.DOTALL).strip()


def render(highlights: str, prs: Sequence[tuple[int, str]], changelog: str) -> str:
    parts = [f"## Highlights\n\n{highlights}\n"]
    if prs:
        listed = "\n".join(f"- {title} (#{number})" for number, title in prs)
        parts.append(f"## Pull requests\n\n{listed}\n")
    parts.append(f"## Changelog\n\n{changelog}\n")
    return "\n".join(parts)


def build(version: str, changelog_path: Path) -> tuple[Path, bool]:
    path = RELEASE_DIR / f"{version}.md"
    highlights = existing_highlights(path)
    drafted = not prose(highlights or "")
    since = previous_tag(version)
    prs = pull_requests(since)
    changelog = changelog_section.section(
        changelog_path.read_text(encoding="utf-8"), version
    )
    RELEASE_DIR.mkdir(parents=True, exist_ok=True)
    path.write_text(
        render(highlights or HIGHLIGHTS_PLACEHOLDER, prs, changelog),
        encoding="utf-8",
    )
    print(
        f"{path.relative_to(REPO_ROOT)}: {len(prs)} pull request(s) since "
        f"{since or 'the first commit'}",
        file=sys.stderr,
    )
    if drafted:
        print(
            "Highlights are a placeholder - write them before publishing.",
            file=sys.stderr,
        )
    else:
        print("Kept the existing Highlights.", file=sys.stderr)
    return path, drafted


def check(version: str) -> int:
    """Fail if the notes are missing or still hold the placeholder."""
    path = RELEASE_DIR / f"{version}.md"
    if not path.exists():
        print(
            f"No release notes at {path.relative_to(REPO_ROOT)}.\n"
            f"Run: make release-notes VERSION={version}",
            file=sys.stderr,
        )
        return 1
    if not prose(existing_highlights(path) or ""):
        print(
            f"{path.relative_to(REPO_ROOT)} still has placeholder Highlights.\n"
            "Write the release story before publishing.",
            file=sys.stderr,
        )
        return 1
    return 0


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="release version, for example v1.20.0")
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify the notes exist and the Highlights are written",
    )
    parser.add_argument(
        "--changelog",
        type=Path,
        default=changelog_section.CHANGELOG,
        help="changelog to read (default: the repo's CHANGELOG.md)",
    )
    args = parser.parse_args(argv)
    if not VERSION_RE.fullmatch(args.version):
        raise SystemExit(f"invalid version {args.version!r}; expected a vX.Y.Z tag")
    if args.check:
        return check(args.version)
    build(args.version, args.changelog)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
