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
    def __init__(
        self, responses: dict[str, bytes], content_type: str = "application/xml"
    ) -> None:
        self.responses = responses
        self.content_type = content_type
        self.requested: list[str] = []

    def request(self, url: str, **_: object) -> tuple[bytes, str, str]:
        self.requested.append(url)
        return self.responses[url], self.content_type, url


class FakeJSONClient:
    def __init__(self, payload: object) -> None:
        self.payload = payload

    def json(self, url: str, **_: object) -> object:
        return self.payload


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


class OpenRouterCollectionTests(unittest.TestCase):
    def test_prefixes_cover_every_namespace_dive_exports(self) -> None:
        repo_root = Path(__file__).resolve().parents[1]
        catalog = provider_watch.load_provider_catalog(repo_root, "openrouter")
        prefixes = tuple(provider_watch.openrouter_prefixes(repo_root))

        namespaced = [
            str(model["id"])
            for model in catalog["models"]
            if "/" in str(model.get("id", ""))
        ]
        self.assertTrue(namespaced)
        for model_id in namespaced:
            self.assertTrue(
                model_id.startswith(prefixes),
                f"{model_id} would be filtered out of the OpenRouter snapshot",
            )

    def test_collect_keeps_models_in_the_requested_namespaces(self) -> None:
        client = FakeJSONClient(
            {
                "data": [
                    {"id": "mistralai/mistral-large-2512", "context_length": 128000},
                    {"id": "cohere/command", "context_length": 4096},
                ]
            }
        )

        result = provider_watch.collect_openrouter_api(client, None, ("mistralai/",))

        self.assertEqual(list(result["models"]), ["mistralai/mistral-large-2512"])


class RedactionTests(unittest.TestCase):
    def test_redact_url_masks_credential_query_parameters(self) -> None:
        masked = provider_watch.redact_url(
            "https://example.test/models?key=super-secret&pageSize=1000"
        )

        self.assertNotIn("super-secret", masked)
        self.assertIn("pageSize=1000", masked)

    def test_fetch_failure_hides_percent_encoded_google_key(self) -> None:
        key = "abc+def/ghi="
        url = provider_watch.query_url(
            "https://generativelanguage.googleapis.com/v1beta/models",
            {"key": key, "pageSize": 1000},
        )
        self.assertNotIn(key, url)  # urlencode escaped it, so a literal replace misses

        client = provider_watch.HTTPClient(retries=0)
        with (
            mock.patch.dict(os.environ, {"GEMINI_API_KEY": key}),
            mock.patch.object(
                provider_watch, "urlopen", side_effect=OSError("connection reset")
            ),
        ):
            with self.assertRaises(provider_watch.FetchFailure) as raised:
                client.request(url)
            rendered = provider_watch.safe_error(raised.exception)

        self.assertNotIn(key, rendered)
        self.assertNotIn("abc%2Bdef", rendered)
        self.assertIn("[redacted]", rendered)


class SnapshotResilienceTests(unittest.TestCase):
    def test_unparsable_discovery_pattern_degrades_one_source(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            provider = root / "providers" / "openai"
            provider.mkdir(parents=True)
            (provider / "catalog.json").write_text(
                """{
  "schema_version": 1,
  "provider": "openai",
  "sources": [
    {"name": "sitemap", "url": "https://example.test/sitemap.xml", "kind": "sitemap",
     "discovery_patterns": ["models("]},
    {"name": "docs", "url": "https://example.test/docs", "kind": "document"}
  ],
  "models": [{"go_name": "ModelNew", "id": "gpt-new", "default": true}],
  "pricing": {}
}
""",
                encoding="utf-8",
            )
            client = FakeClient(
                {
                    "https://example.test/sitemap.xml": (
                        b"<urlset><url><loc>https://example.test/models</loc></url></urlset>"
                    ),
                    "https://example.test/docs": b"GPT pricing: $1 per million tokens",
                }
            )

            result = provider_watch.build_snapshot(
                client=client,
                providers=("openai",),
                public_only=True,
                include_local_ollama=False,
                ollama_url="http://localhost:11434",
                repo_root=root,
                check_sdks=False,
            )

        documents = result["providers"]["openai"]["documents"]
        self.assertEqual(documents["sitemap"]["status"], "error")
        self.assertTrue(documents["sitemap"]["error"])
        # The healthy source in the same provider still lands in the snapshot.
        self.assertEqual(documents["docs"]["status"], "ok")

    def test_describe_error_names_unexpected_failure_types(self) -> None:
        self.assertEqual(
            provider_watch.describe_error(ValueError("boom")), "ValueError: boom"
        )
        self.assertEqual(
            provider_watch.describe_error(provider_watch.FetchFailure("boom")), "boom"
        )


class FeatureTokenTests(unittest.TestCase):
    def test_capitalized_beta_headers_are_collected_and_normalized(self) -> None:
        client = FakeClient(
            {
                "https://example.test/beta": (
                    b"<html><body><p>Set the Interleaved-Thinking-2026-08-08 beta "
                    b"header to enable extended thinking with tools.</p></body></html>"
                )
            },
            content_type="text/html",
        )
        source = provider_watch.DocumentSource(
            provider="anthropic", name="beta", url="https://example.test/beta"
        )

        document = provider_watch.collect_document(client, source)

        self.assertIn("interleaved-thinking-2026-08-08", document["feature_tokens"])


class ReportSizeTests(unittest.TestCase):
    def test_report_stays_within_the_github_issue_body_limit(self) -> None:
        # normalize_lines caps a signal at 600 characters and escape_markdown
        # expands punctuation up to six-fold, so a handful of document changes
        # is enough to blow past GitHub's limit without a byte budget.
        signal = "model-name.v2 {beta} " * 30
        changes = [
            {
                "provider": "anthropic",
                "kind": "document_changed",
                "document": f"docs/page-{index}",
                "added": [f"{signal} #{item}" for item in range(25)],
                "removed": [f"{signal} -{item}" for item in range(25)],
            }
            for index in range(provider_watch.MAX_REPORT_CHANGES)
        ]
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": changes,
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertLessEqual(len(body), provider_watch.MAX_REPORT_BYTES)
        self.assertIn("# Dive provider watch report", body)
        self.assertIn("line(s) omitted", body)
        # The summary table survives truncation, so nothing goes unreported.
        self.assertEqual(
            body.count("| anthropic | `document_changed` |"),
            provider_watch.MAX_REPORT_CHANGES,
        )

    def test_report_stays_bounded_when_subjects_are_hostile(self) -> None:
        # Every table row is provider-controlled text too, so the table alone
        # must not be able to blow the budget.
        subject = "model.name-{beta}" * 40
        changes = [
            {
                "provider": "openai",
                "kind": "model_added",
                "model": f"{subject}-{index}",
                "added": [subject] * 25,
            }
            for index in range(provider_watch.MAX_REPORT_CHANGES)
        ]
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": changes,
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertLessEqual(len(body), provider_watch.MAX_REPORT_BYTES)
        self.assertIn("Detected **100** material change(s).", body)
        self.assertIn("line(s) omitted", body)


class CatalogGapTests(unittest.TestCase):
    """Completeness checks.

    A drift diff only ever sees models that changed between two runs, so a model
    that has been published upstream all along and was never added to Dive stays
    invisible forever. That is how claude-opus-5 was missed, and these tests pin
    the behaviour that catches the next one.
    """

    def gap_snapshot(
        self,
        *,
        model_tokens: list[str],
        listed: list[str],
        api_status: str = "skipped",
        api_models: dict[str, object] | None = None,
        provider: str = "anthropic",
    ) -> dict[str, object]:
        providers = {
            provider: {
                "api": {"status": api_status, "models": api_models or {}},
                "documents": {
                    "pricing": {"status": "ok", "model_tokens": model_tokens}
                },
            }
        }
        repo = {"providers": {provider: {"models": listed}}}
        return {
            "schema_version": provider_watch.SCHEMA_VERSION,
            "generated_at": "2026-08-08T00:00:00+00:00",
            "providers": providers,
            "repo": repo,
            "gaps": provider_watch.catalog_gaps(providers, repo),
        }

    def test_documents_alone_surface_a_model_dive_never_listed(self) -> None:
        # The scheduled audit runs --public-only, so every provider except
        # OpenRouter reports api.status == "skipped" and scraped pages are the
        # only evidence there is. Gap detection has to work from them alone.
        snapshot = self.gap_snapshot(
            model_tokens=["claude-opus-5", "claude-opus-4-8"],
            listed=["claude-opus-4-8"],
        )

        self.assertEqual(snapshot["gaps"], {"anthropic": ["claude-opus-5"]})

    def test_prose_slugs_and_urls_are_not_mistaken_for_models(self) -> None:
        snapshot = self.gap_snapshot(
            model_tokens=[
                "claude-opus-5",
                "Claude",  # prose
                "claude-code",  # no version digit
                "CLAUDE_OPUS_5",  # constant name
                "claude.com/docs/en/about-claude/pricing",  # url
                "anthropic/v1/messages",  # api path
            ],
            listed=["claude-opus-4-8"],
        )

        self.assertEqual(snapshot["gaps"], {"anthropic": ["claude-opus-5"]})

    def test_documentation_links_do_not_file_a_second_phantom_gap(self) -> None:
        # OpenAI's model index links each model as "<id>.md", so the real id and
        # its documentation filename both reach gap detection. Reporting both
        # doubles every release's findings and asks a reviewer to catalog a page.
        snapshot = self.gap_snapshot(
            model_tokens=["gpt-6-astra", "gpt-6-astra.md"],
            listed=["gpt-5.6-sol"],
            provider="openai",
        )

        self.assertEqual(snapshot["gaps"], {"openai": ["gpt-6-astra"]})

    def test_dated_variants_of_listed_models_are_not_gaps(self) -> None:
        snapshot = self.gap_snapshot(
            model_tokens=["claude-opus-4-8", "claude-opus-4-8-20260115"],
            listed=["claude-opus-4-8"],
        )

        self.assertEqual(snapshot["gaps"], {})

    def test_only_newly_appearing_gaps_are_reported(self) -> None:
        # The retired-model tail lives in the accepted baseline so it never files
        # an issue twice; a model that shows up after the baseline does.
        before = self.gap_snapshot(
            model_tokens=["claude-2.1"], listed=["claude-opus-4-8"]
        )
        after = self.gap_snapshot(
            model_tokens=["claude-2.1", "claude-opus-5"], listed=["claude-opus-4-8"]
        )

        diff = provider_watch.diff_snapshots(before, after)

        gaps = [
            change
            for change in diff["changes"]
            if change["kind"] == "model_missing_from_dive"
        ]
        self.assertEqual([change["model"] for change in gaps], ["claude-opus-5"])
        self.assertEqual(gaps[0]["sources"], ["document:pricing"])

    def test_adding_the_model_to_the_catalog_clears_the_gap(self) -> None:
        before = self.gap_snapshot(
            model_tokens=["claude-opus-5"], listed=["claude-opus-4-8"]
        )
        after = self.gap_snapshot(
            model_tokens=["claude-opus-5"],
            listed=["claude-opus-4-8", "claude-opus-5"],
        )

        diff = provider_watch.diff_snapshots(before, after)

        self.assertEqual(after["gaps"], {})
        self.assertFalse(
            [c for c in diff["changes"] if c["kind"] == "model_missing_from_dive"]
        )

    def test_baseline_without_gap_data_does_not_report_the_backlog(self) -> None:
        before = self.gap_snapshot(
            model_tokens=["claude-opus-5"], listed=["claude-opus-4-8"]
        )
        del before["gaps"]
        after = self.gap_snapshot(
            model_tokens=["claude-opus-5"], listed=["claude-opus-4-8"]
        )

        diff = provider_watch.diff_snapshots(before, after)

        self.assertFalse(
            [c for c in diff["changes"] if c["kind"] == "model_missing_from_dive"]
        )

    def test_provider_without_a_catalog_is_not_reported_as_all_gaps(self) -> None:
        providers = {
            "anthropic": {
                "api": {"status": "ok", "models": {"claude-opus-5": {}}},
                "documents": {},
            }
        }

        gaps = provider_watch.catalog_gaps(providers, {"providers": {}})

        self.assertEqual(gaps, {})

    def test_xai_prefixed_ids_are_normalized_before_comparison(self) -> None:
        providers = {
            "xai": {
                "api": {"status": "ok", "models": {"xai:grok-4.5": {}}},
                "documents": {},
            }
        }
        repo = {"providers": {"xai": {"models": ["grok-4.5"]}}}

        self.assertEqual(provider_watch.catalog_gaps(providers, repo), {})

    def test_report_leads_with_missing_models(self) -> None:
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": [
                {
                    "provider": "anthropic",
                    "kind": "document_changed",
                    "document": "pricing",
                },
                {
                    "provider": "anthropic",
                    "kind": "model_missing_from_dive",
                    "model": "claude-opus-5",
                    "sources": ["document:pricing"],
                    "dive": {"provider_catalog": False, "cli_recommended": False},
                },
            ],
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertIn("1 model(s) published upstream are missing", body)
        rows = [line for line in body.splitlines() if line.startswith("| anthropic")]
        self.assertIn("model_missing_from_dive", rows[0])
        self.assertIn("no entry in `providers/anthropic/catalog.json`", body)


class UnverifiedModelTests(unittest.TestCase):
    """The inverse of the gap check: ids Dive ships that upstream never mentions.

    Six Mistral constants and nine OpenRouter constants shipped ids their
    providers do not serve. Every one would have failed at the API, and nothing
    in the watcher looked in this direction.
    """

    def provider(
        self,
        *,
        signals: list[str],
        api_models: dict[str, object] | None = None,
    ) -> dict[str, object]:
        return {
            "api": (
                {"status": "ok", "models": api_models}
                if api_models is not None
                else {"status": "skipped", "models": {}}
            ),
            "documents": {"models": {"status": "ok", "signals": signals}},
        }

    def test_fabricated_id_is_reported(self) -> None:
        providers = {
            "mistral": self.provider(
                signals=[
                    "We released Mistral Large 3 (mistral-large-2512).",
                    "mistral-large-latest",
                    "mistral-large-2411",
                ]
            )
        }
        repo = {
            "providers": {
                "mistral": {
                    "models": [
                        "mistral-large-2412",
                        "mistral-large-latest",
                        "mistral-large-2411",
                        "mistral-large-2512",
                    ]
                }
            }
        }

        self.assertEqual(
            provider_watch.unverified_models(providers, repo),
            {"mistral": ["mistral-large-2412"]},
        )

    def test_a_longer_id_is_not_evidence_for_a_shorter_one(self) -> None:
        # devstral-2512 must not vouch for the fabricated devstral-2, which is
        # precisely the substring trap this check has to avoid.
        providers = {"mistral": self.provider(signals=["devstral-2512 is available"])}
        repo = {"providers": {"mistral": {"models": ["devstral-2", "devstral-2512"]}}}

        self.assertEqual(
            provider_watch.unverified_models(providers, repo),
            {"mistral": ["devstral-2"]},
        )

    def test_dated_snapshot_confirms_the_undated_alias(self) -> None:
        # Providers document claude-haiku-4-5-20251001 while serving the alias.
        providers = {
            "anthropic": self.provider(signals=["claude-haiku-4-5-20251001 | Active"])
        }
        repo = {"providers": {"anthropic": {"models": ["claude-haiku-4-5"]}}}

        self.assertEqual(provider_watch.unverified_models(providers, repo), {})

    def test_live_api_listing_settles_separator_spelling(self) -> None:
        # OpenRouter serves anthropic/claude-opus-4.7; Dive shipped the dashed
        # spelling, which the API does not resolve.
        providers = {
            "openrouter": self.provider(
                signals=[],
                api_models={
                    "anthropic/claude-opus-4.7": {},
                    "anthropic/claude-fable-5": {},
                },
            )
        }
        repo = {
            "providers": {
                "openrouter": {
                    "models": ["anthropic/claude-opus-4-7", "anthropic/claude-fable-5"]
                }
            }
        }

        self.assertEqual(
            provider_watch.unverified_models(providers, repo),
            {"openrouter": ["anthropic/claude-opus-4-7"]},
        )

    def test_sources_that_do_not_enumerate_models_report_nothing(self) -> None:
        # Ollama's pages document the API, not the library, and corroborate
        # almost nothing. Their silence must not condemn the whole catalog.
        providers = {"ollama": self.provider(signals=["gpt-oss is supported"])}
        repo = {
            "providers": {
                "ollama": {
                    "models": [
                        "gpt-oss",
                        "qwen3.6",
                        "gemma4",
                        "deepseek-r1",
                        "glm-4.7-flash",
                    ]
                }
            }
        }

        self.assertEqual(provider_watch.unverified_models(providers, repo), {})

    def test_models_the_catalog_marks_retired_are_not_reported(self) -> None:
        # Upstream dropping a retired model from its docs is the expected
        # outcome, not a finding the catalog needs to answer for.
        providers = {"openrouter": self.provider(signals=["google/gemini-3.1-pro-preview"])}
        repo = {
            "providers": {
                "openrouter": {
                    "models": [
                        "google/gemini-3.1-pro-preview",
                        "google/gemini-3-pro-preview",
                    ],
                    "retired": ["google/gemini-3-pro-preview"],
                }
            }
        }

        self.assertEqual(provider_watch.unverified_models(providers, repo), {})

    def test_only_newly_unverified_ids_are_reported(self) -> None:
        def snapshot(models: list[str]) -> dict[str, object]:
            providers = {
                "mistral": self.provider(
                    signals=[
                        "mistral-large-2512",
                        "mistral-small-latest",
                        "codestral-latest",
                    ]
                )
            }
            repo = {"providers": {"mistral": {"models": models}}}
            return {
                "providers": providers,
                "repo": repo,
                "gaps": {},
                "unverified": provider_watch.unverified_models(providers, repo),
            }

        # open-mistral-7b is a retired id the baseline already accepts.
        corroborated = [
            "mistral-large-2512",
            "mistral-small-latest",
            "codestral-latest",
        ]
        before = snapshot([*corroborated, "open-mistral-7b"])
        after = snapshot([*corroborated, "open-mistral-7b", "devstral-2"])

        diff = provider_watch.diff_snapshots(before, after)

        reported = [
            change["model"]
            for change in diff["changes"]
            if change["kind"] == "model_not_found_upstream"
        ]
        self.assertEqual(reported, ["devstral-2"])

    def test_report_leads_with_unserved_ids(self) -> None:
        diff = {
            "generated_at": "2026-08-08T00:00:00+00:00",
            "change_hash": "abc123",
            "changes": [
                {
                    "provider": "anthropic",
                    "kind": "model_missing_from_dive",
                    "model": "claude-opus-6",
                },
                {
                    "provider": "mistral",
                    "kind": "model_not_found_upstream",
                    "model": "devstral-2",
                },
            ],
        }

        body = provider_watch.report_markdown(diff, {"repo": {"providers": {}}})

        self.assertIn("1 model id(s) Dive ships are not served", body)
        self.assertIn("1 model(s) published upstream are missing", body)
        rows = [line for line in body.splitlines() if line.startswith("| ")]
        self.assertIn("model_not_found_upstream", rows[2])


class ModelTokenTests(unittest.TestCase):
    def test_mistral_model_families_are_covered(self) -> None:
        # Ministral, Magistral, Pixtral, and Voxtral ids were invisible to the
        # tokenizer, so gap detection could never report one as missing. A new
        # "-tral" family needs adding to MODEL_TOKEN_RE and to this list.
        published = [
            "mistral-large-2512",
            "mixtral-8x22b",
            "ministral-14b-2512",
            "magistral-medium-2509",
            "codestral-2508",
            "devstral-2512",
            "pixtral-large-2411",
            "voxtral-mini-2507",
        ]

        for model_id in published:
            with self.subTest(model=model_id):
                self.assertEqual(
                    provider_watch.MODEL_TOKEN_RE.findall(f"see {model_id} today"),
                    [model_id],
                )

    def test_muse_model_families_are_covered(self) -> None:
        # The scheduled audit runs --public-only, so scraped pages are the only
        # evidence for Meta. A Muse id this pattern does not recognize is one
        # the watcher can never report as missing from Dive's catalog.
        for model_id in (
            "muse-spark-1.3",
            "muse-spark-1.3-contributor",
            "muse-spark-1.2",
            "muse-image-1.0",
            "muse-voice-transcribe-1.0",
        ):
            with self.subTest(model=model_id):
                self.assertEqual(
                    provider_watch.MODEL_TOKEN_RE.findall(f"see {model_id} today"),
                    [model_id],
                )

    def test_meta_is_registered_end_to_end(self) -> None:
        # Registration is spread across four tables; missing any one of them
        # leaves the provider silently unwatched rather than erroring.
        self.assertIn("meta", provider_watch.PROVIDERS)
        self.assertEqual(provider_watch.CATALOG_DIRECTORIES["meta"], "meta")
        self.assertIn("meta", provider_watch.API_COLLECTORS)
        env_names, _ = provider_watch.API_COLLECTORS["meta"]
        # MODEL_API_KEY is the name Meta's own docs use; the namespaced alias
        # exists because that name is generic enough to already be taken.
        self.assertEqual(env_names, ("MODEL_API_KEY", "META_API_KEY"))
        for name in env_names:
            self.assertIn(name, provider_watch.API_KEY_ENV_NAMES)

    def test_open_and_labs_prefixes_stay_attached(self) -> None:
        # Truncating these produces an id the API does not accept, which then
        # reads as a gap against the correctly-spelled catalog entry.
        for model_id in (
            "open-mistral-7b",
            "open-codestral-mamba",
            "labs-devstral-small-2512",
        ):
            with self.subTest(model=model_id):
                self.assertEqual(
                    provider_watch.MODEL_TOKEN_RE.findall(f"use {model_id} here"),
                    [model_id],
                )


class CheckedInBaselineTests(unittest.TestCase):
    def test_baseline_carries_accepted_gaps_for_the_current_schema(self) -> None:
        # Without this the first scheduled run after the schema bump would file
        # an issue listing every historical model id at once.
        path = SCRIPT.with_name("provider_watch_baseline.json.gz")
        baseline = provider_watch.load_json(path)

        self.assertEqual(baseline["schema_version"], provider_watch.SCHEMA_VERSION)
        self.assertIn("gaps", baseline)
        self.assertNotIn(
            "claude-opus-5", baseline["gaps"].get("anthropic", [])
        )


if __name__ == "__main__":
    unittest.main()
