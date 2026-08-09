#!/usr/bin/env python3
"""Unit tests for the provider-watch prototype."""

from __future__ import annotations

import gzip
import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("provider_watch.py")
SPEC = importlib.util.spec_from_file_location("provider_watch", SCRIPT)
assert SPEC and SPEC.loader
provider_watch = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = provider_watch
SPEC.loader.exec_module(provider_watch)


def snapshot(provider: dict[str, object]) -> dict[str, object]:
    return {
        "schema_version": 1,
        "generated_at": "2026-08-08T00:00:00+00:00",
        "providers": {"openai": provider},
        "repo": {"providers": {}},
    }


class FakeClient:
    def __init__(self, responses: dict[str, bytes]) -> None:
        self.responses = responses
        self.requested: list[str] = []

    def request(self, url: str, **_: object) -> tuple[bytes, str, str]:
        self.requested.append(url)
        return self.responses[url], "application/xml", url


class FakeResponse:
    def __init__(self, data: bytes, url: str = "https://example.test/data") -> None:
        self.data = data
        self.url = url
        self.headers = mock.Mock()
        self.headers.get_content_type.return_value = "application/octet-stream"

    def __enter__(self) -> "FakeResponse":
        return self

    def __exit__(self, *_: object) -> None:
        return None

    def read(self, size: int) -> bytes:
        return self.data[:size]

    def geturl(self) -> str:
        return self.url


class TextExtractionTests(unittest.TestCase):
    def test_visible_text_prefers_main_and_skips_scripts(self) -> None:
        page = b"""
            <html><body>
              <nav>Old pricing: $999</nav>
              <main><h1>GPT-7 pricing</h1><p>Input: $1 per million tokens</p>
              <script>secret model gpt-bogus</script></main>
            </body></html>
        """

        text = provider_watch.visible_text(page, "text/html", "https://example.test/")

        self.assertIn("GPT-7 pricing", text)
        self.assertIn("$1 per million tokens", text)
        self.assertNotIn("$999", text)
        self.assertNotIn("gpt-bogus", text)

    def test_sitemap_index_expands_child_sitemaps(self) -> None:
        root = b"""<?xml version="1.0"?>
          <sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
            <sitemap><loc>https://example.test/docs.xml</loc></sitemap>
          </sitemapindex>"""
        child = b"""<?xml version="1.0"?>
          <urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
            <url><loc>https://example.test/models</loc></url>
            <url><loc>https://example.test/unrelated</loc></url>
          </urlset>"""
        client = FakeClient({"https://example.test/docs.xml": child})

        links = provider_watch.collect_sitemap_links(client, root)

        self.assertIn("https://example.test/models", links)
        self.assertEqual(client.requested, ["https://example.test/docs.xml"])

    def test_sitemap_index_expands_gzipped_child_sitemaps(self) -> None:
        root = b"""<?xml version="1.0"?>
          <sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
            <sitemap><loc>https://example.test/docs.xml.gz</loc></sitemap>
          </sitemapindex>"""
        child = b"""<?xml version="1.0"?>
          <urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
            <url><loc>https://example.test/models</loc></url>
          </urlset>"""
        client = FakeClient({"https://example.test/docs.xml.gz": gzip.compress(child)})

        links = provider_watch.collect_sitemap_links(client, root)

        self.assertIn("https://example.test/models", links)


class HTTPClientTests(unittest.TestCase):
    def test_request_rejects_unsupported_url_scheme(self) -> None:
        client = provider_watch.HTTPClient(retries=3)
        with mock.patch.object(provider_watch, "urlopen") as urlopen:
            with self.assertRaisesRegex(provider_watch.FetchFailure, "unsupported URL scheme"):
                client.request("file:///etc/passwd")

        urlopen.assert_not_called()

    def test_request_does_not_retry_oversize_response(self) -> None:
        response = FakeResponse(b"x" * (provider_watch.MAX_RESPONSE_BYTES + 1))
        client = provider_watch.HTTPClient(retries=3)
        with (
            mock.patch.object(provider_watch, "urlopen", return_value=response) as urlopen,
            mock.patch.object(provider_watch.time, "sleep") as sleep,
        ):
            with self.assertRaisesRegex(provider_watch.FetchFailure, "response exceeded"):
                client.request("https://example.test/data")

        self.assertEqual(urlopen.call_count, 1)
        sleep.assert_not_called()


class DiffTests(unittest.TestCase):
    def test_diff_classifies_catalog_pricing_and_document_changes(self) -> None:
        before = snapshot(
            {
                "api": {
                    "status": "ok",
                    "models": {
                        "gpt-old": {"pricing": {"prompt": "1"}},
                        "gpt-stable": {"context_length": 100},
                    },
                },
                "documents": {
                    "deprecations": {
                        "status": "ok",
                        "url": "https://example.test/deprecations",
                        "semantic_sha256": "old",
                        "signals": ["GPT-old is supported"],
                    }
                },
            }
        )
        after = snapshot(
            {
                "api": {
                    "status": "ok",
                    "models": {
                        "gpt-old": {"pricing": {"prompt": "2"}},
                        "gpt-stable": {"context_length": 100},
                        "gpt-new": {"context_length": 200},
                    },
                },
                "documents": {
                    "deprecations": {
                        "status": "ok",
                        "url": "https://example.test/deprecations",
                        "semantic_sha256": "new",
                        "signals": ["GPT-old is deprecated; replacement is GPT-new"],
                    }
                },
            }
        )

        result = provider_watch.diff_snapshots(before, after)
        kinds = [change["kind"] for change in result["changes"]]

        self.assertTrue(result["has_changes"])
        self.assertIn("model_added", kinds)
        self.assertIn("pricing_changed", kinds)
        self.assertIn("lifecycle_changed", kinds)

    def test_raw_document_churn_is_not_material(self) -> None:
        document = {
            "status": "ok",
            "url": "https://example.test/models",
            "semantic_sha256": "same",
            "signals": ["GPT-7 model"],
        }
        before = snapshot(
            {"api": {"status": "skipped", "models": {}}, "documents": {"models": {**document, "raw_sha256": "a"}}}
        )
        after = snapshot(
            {"api": {"status": "skipped", "models": {}}, "documents": {"models": {**document, "raw_sha256": "b"}}}
        )

        result = provider_watch.diff_snapshots(before, after)

        self.assertFalse(result["has_changes"])
        self.assertIsNone(result["change_hash"])

    def test_beta_header_change_is_classified_as_feature_flag(self) -> None:
        kind = provider_watch.classify_document_change(
            ["Enable interleaved-thinking-2026-08-08 for Claude"], []
        )

        self.assertEqual(kind, "feature_flag_changed")

    def test_document_diff_marks_unlisted_feature_flags(self) -> None:
        before = snapshot(
            {
                "api": {"status": "skipped", "models": {}},
                "documents": {
                    "beta": {
                        "status": "ok",
                        "url": "https://example.test/beta",
                        "semantic_sha256": "old",
                        "signals": [],
                        "feature_tokens": [],
                    }
                },
            }
        )
        after = snapshot(
            {
                "api": {"status": "skipped", "models": {}},
                "documents": {
                    "beta": {
                        "status": "ok",
                        "url": "https://example.test/beta",
                        "semantic_sha256": "new",
                        "signals": ["Enable tools-2026-08-08 beta"],
                        "feature_tokens": ["tools-2026-08-08"],
                    }
                },
            }
        )

        result = provider_watch.diff_snapshots(before, after)

        change = result["changes"][0]
        self.assertEqual(change["kind"], "feature_flag_changed")
        self.assertEqual(change["unlisted_feature_flags"], ["tools-2026-08-08"])

    def test_change_hash_is_stable_across_generation_times(self) -> None:
        before = snapshot(
            {"api": {"status": "ok", "models": {}}, "documents": {}}
        )
        after = snapshot(
            {
                "api": {"status": "ok", "models": {"gpt-new": {"id": "gpt-new"}}},
                "documents": {},
            }
        )
        first = provider_watch.diff_snapshots(before, after)
        after["generated_at"] = "2026-08-15T00:00:00+00:00"
        second = provider_watch.diff_snapshots(before, after)

        self.assertEqual(first["change_hash"], second["change_hash"])

    def test_provider_release_metadata_is_diffed(self) -> None:
        before = {
            "schema_version": 1,
            "providers": {
                "ollama": {
                    "api": {"status": "skipped", "models": {}},
                    "documents": {},
                    "metadata": {
                        "releases": {
                            "status": "ok",
                            "releases": [{"tag_name": "v1.0.0"}],
                        }
                    },
                }
            },
        }
        after = {
            **before,
            "providers": {
                "ollama": {
                    **before["providers"]["ollama"],
                    "metadata": {
                        "releases": {
                            "status": "ok",
                            "releases": [{"tag_name": "v1.1.0"}],
                        }
                    },
                }
            },
        }

        result = provider_watch.diff_snapshots(before, after)

        self.assertEqual(result["changes"][0]["kind"], "release_changed")

    def test_source_degradation_records_error_details(self) -> None:
        before = snapshot(
            {"api": {"status": "ok", "models": {}}, "documents": {}}
        )
        after = snapshot(
            {
                "api": {"status": "error", "error": "upstream unavailable", "models": {}},
                "documents": {},
            }
        )

        result = provider_watch.diff_snapshots(before, after)

        self.assertEqual(result["changes"][0]["kind"], "source_error")
        self.assertEqual(result["changes"][0]["details"], "upstream unavailable")

    def test_model_removal_is_reported(self) -> None:
        before = snapshot(
            {"api": {"status": "ok", "models": {"gpt-old": {"id": "gpt-old"}}}, "documents": {}}
        )
        after = snapshot(
            {"api": {"status": "ok", "models": {}}, "documents": {}}
        )

        result = provider_watch.diff_snapshots(before, after)

        self.assertEqual(result["changes"][0]["kind"], "model_removed")
        self.assertEqual(result["changes"][0]["model"], "gpt-old")

    def test_document_source_addition_and_removal_are_reported(self) -> None:
        old_document = {
            "status": "ok",
            "url": "https://example.test/old",
            "semantic_sha256": "old",
        }
        new_document = {
            "status": "ok",
            "url": "https://example.test/new",
            "semantic_sha256": "new",
        }
        before = snapshot(
            {
                "api": {"status": "skipped", "models": {}},
                "documents": {"old": old_document},
            }
        )
        after = snapshot(
            {
                "api": {"status": "skipped", "models": {}},
                "documents": {"new": new_document},
            }
        )

        result = provider_watch.diff_snapshots(before, after)
        changes = {change["kind"]: change for change in result["changes"]}

        self.assertEqual(changes["document_source_added"]["document"], "new")
        self.assertEqual(changes["document_source_removed"]["document"], "old")


class ReportMarkdownTests(unittest.TestCase):
    def test_report_renders_and_escapes_provider_text(self) -> None:
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": [
                {
                    "provider": "openai",
                    "kind": "document_changed",
                    "document": "models`|<img>",
                    "details": "[click](https://example.test)<script>",
                    "added": ["new | `signal` <img>"],
                }
            ],
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertIn("# Dive provider watch report", body)
        self.assertIn("models&#96;&#124;&lt;img&gt;", body)
        self.assertIn("&#91;click&#93;&#40;https://example&#46;test&#41;&lt;script&gt;", body)
        self.assertNotIn("<img>", body)
        self.assertNotIn("<script>", body)

    def test_report_truncates_github_issue_body(self) -> None:
        changes = [
            {"provider": "openai", "kind": "model_added", "model": f"model-{index}"}
            for index in range(provider_watch.MAX_REPORT_CHANGES + 1)
        ]
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": changes,
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertIn(
            f"Showing the first {provider_watch.MAX_REPORT_CHANGES} changes", body
        )
        self.assertEqual(body.count("\n## openai:"), provider_watch.MAX_REPORT_CHANGES)
        self.assertNotIn(
            f"model&#45;{provider_watch.MAX_REPORT_CHANGES}</code>", body
        )


class RepoInspectionTests(unittest.TestCase):
    def test_collect_repo_facts_reads_catalog_defaults_aliases_and_pricing(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            provider = root / "providers" / "openai"
            provider.mkdir(parents=True)
            (provider / "catalog.json").write_text(
                """{
  "schema_version": 1,
  "provider": "openai",
  "sources": [{"name": "models", "url": "https://example.test/models"}],
  "models": [
    {
      "go_name": "ModelNew",
      "id": "gpt-new",
      "default": true,
      "recommended": true
    },
    {"go_name": "ModelAlias", "alias_of": "ModelNew"}
  ],
  "pricing": {
    "text": [{
      "model": "gpt-new",
      "input_price_per_1m_tokens": "1.25",
      "output_price_per_1m_tokens": "5.00",
      "currency": "USD",
      "updated_at": "2026-08-08"
    }]
  }
}
""",
                encoding="utf-8",
            )

            facts = provider_watch.collect_repo_facts(root)

        openai = facts["providers"]["openai"]
        self.assertEqual(openai["models"], ["gpt-new"])
        self.assertEqual(openai["defaults"], ["gpt-new"])
        self.assertEqual(
            openai["pricing"]["gpt-new"]["input_price_per_1m_tokens"], "1.25"
        )
        self.assertEqual(facts["cli_model_ids"], ["gpt-new"])

    def test_safe_error_redacts_api_keys(self) -> None:
        with mock.patch.dict(os.environ, {"OPENAI_API_KEY": "top-secret"}):
            rendered = provider_watch.safe_error(
                RuntimeError("request failed with top-secret")
            )
        self.assertEqual(rendered, "request failed with [redacted]")

    def test_build_snapshot_records_unreadable_catalog_per_provider(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            provider = root / "providers" / "openai"
            provider.mkdir(parents=True)
            (provider / "catalog.json").write_text(
                """{
  "schema_version": 1,
  "provider": "openai",
  "sources": [],
  "models": [{"go_name": "ModelNew", "id": "gpt-new", "default": true}],
  "pricing": {}
}
""",
                encoding="utf-8",
            )

            result = provider_watch.build_snapshot(
                client=FakeClient({}),
                providers=("openai", "google"),
                public_only=True,
                include_local_ollama=False,
                ollama_url="http://localhost:11434",
                repo_root=root,
                check_sdks=False,
            )

        self.assertIn("openai", result["providers"])
        self.assertIn("google", result["providers"])
        self.assertIn("failed to read provider catalog", result["errors"]["google"])

    def test_json_gzip_round_trip_is_deterministic(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "snapshot.json.gz"
            provider_watch.write_json(path, {"models": ["gpt-new"]})
            first = path.read_bytes()
            provider_watch.write_json(path, {"models": ["gpt-new"]})

            self.assertEqual(path.read_bytes(), first)
            self.assertEqual(
                provider_watch.load_json(path), {"models": ["gpt-new"]}
            )


if __name__ == "__main__":
    unittest.main()
