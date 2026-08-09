#!/usr/bin/env python3
"""Unit tests for the changelog section extractor."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import sys
import unittest


SCRIPT = Path(__file__).with_name("changelog_section.py")
SPEC = importlib.util.spec_from_file_location("changelog_section", SCRIPT)
assert SPEC and SPEC.loader
changelog_section = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = changelog_section
SPEC.loader.exec_module(changelog_section)


CHANGELOG = """# Changelog

Preamble that is not part of any release.

## [Unreleased]

### Added

- Something not yet cut.

## [1.20.0] - 2026-08-09

### Changed

- **A changed thing.**

### Fixed

- **A fixed thing.**

## [1.19.0] - 2026-08-09

### Added

- An older thing.

## [1.18.0] - 2026-07-22

### Added

- An even older thing.
"""


class SectionTest(unittest.TestCase):
    def test_extracts_named_version(self) -> None:
        body = changelog_section.section(CHANGELOG, "v1.20.0")
        self.assertIn("**A changed thing.**", body)
        self.assertIn("**A fixed thing.**", body)

    def test_stops_at_the_next_version(self) -> None:
        body = changelog_section.section(CHANGELOG, "v1.20.0")
        self.assertNotIn("An older thing", body)
        self.assertNotIn("Something not yet cut", body)

    def test_excludes_the_preamble(self) -> None:
        body = changelog_section.section(CHANGELOG, "v1.19.0")
        self.assertNotIn("Preamble", body)

    def test_accepts_version_with_or_without_v(self) -> None:
        self.assertEqual(
            changelog_section.section(CHANGELOG, "1.20.0"),
            changelog_section.section(CHANGELOG, "v1.20.0"),
        )

    def test_extracts_unreleased(self) -> None:
        body = changelog_section.section(CHANGELOG, "Unreleased")
        self.assertIn("Something not yet cut", body)

    def test_body_is_stripped_of_surrounding_blank_lines(self) -> None:
        body = changelog_section.section(CHANGELOG, "v1.18.0")
        self.assertTrue(body.startswith("### Added"))
        self.assertTrue(body.endswith("An even older thing."))

    def test_missing_version_is_an_error(self) -> None:
        with self.assertRaises(SystemExit) as raised:
            changelog_section.section(CHANGELOG, "v9.9.9")
        self.assertIn("1.20.0", str(raised.exception))

    def test_empty_section_is_an_error(self) -> None:
        empty = "## [1.21.0] - 2026-08-10\n\n## [1.20.0] - 2026-08-09\n\n- Cut.\n"
        with self.assertRaises(SystemExit) as raised:
            changelog_section.section(empty, "v1.21.0")
        self.assertIn("empty", str(raised.exception))

    def test_repo_changelog_parses(self) -> None:
        text = changelog_section.CHANGELOG.read_text(encoding="utf-8")
        versions = [version for version, _ in changelog_section.sections(text)]
        self.assertIn("unreleased", versions)
        self.assertIn("1.19.0", versions)
        # Every heading the repo owns must yield a non-empty body, so a release
        # can never be published with notes the parser silently dropped.
        for version in versions:
            if version == "unreleased":
                continue
            self.assertTrue(changelog_section.section(text, version))


if __name__ == "__main__":
    unittest.main()
