#!/usr/bin/env python3
"""Unit tests for the release-notes assembler."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock


SCRIPTS = Path(__file__).parent
sys.path.insert(0, str(SCRIPTS))

SCRIPT = SCRIPTS / "release_notes.py"
SPEC = importlib.util.spec_from_file_location("release_notes", SCRIPT)
assert SPEC and SPEC.loader
release_notes = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = release_notes
SPEC.loader.exec_module(release_notes)


CHANGELOG = """# Changelog

## [Unreleased]

## [1.20.0] - 2026-08-09

### Changed

- **A changed thing.**
"""


class PullRequestTest(unittest.TestCase):
    def collect(self, *subjects: str) -> list[tuple[int, str]]:
        with mock.patch.object(release_notes, "git", return_value="\n".join(subjects)):
            return release_notes.pull_requests("v1.19.0")

    def test_parses_squash_merge_subjects(self) -> None:
        self.assertEqual(
            self.collect("Do a thing (#246)", "Do another (#245)"),
            [(246, "Do a thing"), (245, "Do another")],
        )

    def test_skips_the_release_prep_commit(self) -> None:
        self.assertEqual(
            self.collect("Prepare v1.19.0 release", "Do a thing (#246)"),
            [(246, "Do a thing")],
        )

    def test_skips_commits_with_no_pr_number(self) -> None:
        self.assertEqual(self.collect("Fix a typo"), [])

    def test_keeps_parenthetical_titles(self) -> None:
        self.assertEqual(
            self.collect("Format Markdown with Prettier (one-time) (#241)"),
            [(241, "Format Markdown with Prettier (one-time)")],
        )


class PreviousTagTest(unittest.TestCase):
    TAGS = "v1.20.0\nv1.19.0\na2a/v1.19.0\nexperimental/cmd/dive/v1.19.0\nv1.18.0"

    def test_ignores_sub_module_tags(self) -> None:
        with mock.patch.object(release_notes, "git", return_value=self.TAGS):
            self.assertEqual(release_notes.previous_tag("v1.20.0"), "v1.19.0")

    def test_excludes_the_version_being_released(self) -> None:
        with mock.patch.object(release_notes, "git", return_value=self.TAGS):
            self.assertEqual(release_notes.previous_tag("v1.19.0"), "v1.20.0")

    def test_no_tags_yet(self) -> None:
        with mock.patch.object(release_notes, "git", return_value=""):
            self.assertIsNone(release_notes.previous_tag("v1.0.0"))


class ProseTest(unittest.TestCase):
    def test_placeholder_has_no_prose(self) -> None:
        self.assertEqual(release_notes.prose(release_notes.HIGHLIGHTS_PLACEHOLDER), "")

    def test_comment_alongside_prose_leaves_prose(self) -> None:
        self.assertEqual(
            release_notes.prose("<!-- a note -->\n\nReal words."), "Real words."
        )


class BuildTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        root = Path(self.tmp.name)
        self.changelog = root / "CHANGELOG.md"
        self.changelog.write_text(CHANGELOG, encoding="utf-8")
        patches = [
            mock.patch.object(release_notes, "REPO_ROOT", root),
            mock.patch.object(release_notes, "RELEASE_DIR", root / "docs" / "releases"),
            mock.patch.object(release_notes, "previous_tag", return_value="v1.19.0"),
            mock.patch.object(
                release_notes, "pull_requests", return_value=[(246, "Do a thing")]
            ),
        ]
        for patch in patches:
            patch.start()
            self.addCleanup(patch.stop)

    def build(self) -> tuple[Path, bool]:
        return release_notes.build("v1.20.0", self.changelog)

    def test_first_build_writes_all_three_sections(self) -> None:
        path, drafted = self.build()
        text = path.read_text(encoding="utf-8")
        self.assertTrue(drafted)
        self.assertIn("## Highlights", text)
        self.assertIn("- Do a thing (#246)", text)
        self.assertIn("**A changed thing.**", text)

    def test_check_fails_on_a_placeholder(self) -> None:
        self.build()
        self.assertEqual(release_notes.check("v1.20.0"), 1)

    def test_check_passes_once_highlights_are_written(self) -> None:
        path, _ = self.build()
        path.write_text(
            path.read_text(encoding="utf-8").replace(
                release_notes.HIGHLIGHTS_PLACEHOLDER, "A real story."
            ),
            encoding="utf-8",
        )
        self.assertEqual(release_notes.check("v1.20.0"), 0)

    def test_rebuild_preserves_written_highlights(self) -> None:
        path, _ = self.build()
        path.write_text(
            path.read_text(encoding="utf-8").replace(
                release_notes.HIGHLIGHTS_PLACEHOLDER, "A real story."
            ),
            encoding="utf-8",
        )
        with mock.patch.object(
            release_notes, "pull_requests", return_value=[(246, "Do a thing"), (247, "And another")]
        ):
            path, drafted = self.build()
        text = path.read_text(encoding="utf-8")
        self.assertFalse(drafted)
        self.assertIn("A real story.", text)
        self.assertIn("- And another (#247)", text)
        self.assertNotIn("Replace this comment", text)

    def test_check_fails_when_notes_are_missing(self) -> None:
        self.assertEqual(release_notes.check("v9.9.9"), 1)


if __name__ == "__main__":
    unittest.main()
