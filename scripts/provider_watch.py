#!/usr/bin/env python3
"""Detect upstream model-provider catalog, pricing, and capability drift.

The script deliberately separates observation from Dive's support policy:

* ``snapshot`` fetches public documentation plus optional account-visible APIs
  and writes a deterministic JSON snapshot.
* ``audit`` compares a fresh snapshot with an accepted baseline and writes a
  compact Markdown/JSON report.
* ``diff`` compares two already-created snapshots without network access.

Only Python's standard library is required. API keys are read from the same
environment variables used by Dive and are never written to output.
"""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
import dataclasses
import datetime as dt
import gzip
import hashlib
import html
from html.parser import HTMLParser
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from typing import Any, Callable, Iterable, Mapping, Sequence
import unicodedata
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urljoin, urlparse
from urllib.request import Request, urlopen
import xml.etree.ElementTree as ET


SCHEMA_VERSION = 1
USER_AGENT = "dive-provider-watch/0.1 (+https://github.com/deepnoodle-ai/dive)"
MAX_RESPONSE_BYTES = 8 * 1024 * 1024
DEFAULT_TIMEOUT_SECONDS = 30.0
DEFAULT_RETRIES = 2
MAX_REPORT_CHANGES = 100

PROVIDERS = (
    "openai",
    "anthropic",
    "google",
    "xai",
    "mistral",
    "openrouter",
    "ollama",
)

CATALOG_DIRECTORIES = {
    "openai": "openai",
    "anthropic": "anthropic",
    "google": "google",
    "xai": "grok",
    "mistral": "mistral",
    "openrouter": "openrouter",
    "ollama": "ollama",
}

OPENROUTER_PREFIXES = (
    "anthropic/",
    "deepseek/",
    "google/",
    "mistral/",
    "openai/",
    "x-ai/",
)

MATERIAL_LINE_RE = re.compile(
    r"(?:"
    r"\bmodel(?:s)?\b|\bpricing\b|\bprice\b|\btokens?\b|\bcontext\b|"
    r"\bdeprecat(?:ed|ion|ions)?\b|\bretir(?:ed|ement)\b|\bshutdown\b|"
    r"\breplacement\b|\balias(?:es)?\b|\breasoning\b|\bthinking\b|"
    r"\btemperature\b|\btop[_ -]?[pk]\b|\btools?\b|\bfunction calling\b|"
    r"\bstructured outputs?\b|\bvision\b|\bimage\b|\baudio\b|\bvideo\b|"
    r"\bcach(?:e|ed|ing)\b|\bbatch\b|\bfast mode\b|\bflex\b|\bbeta\b|"
    r"\brelease notes?\b|\$\s*\d|\b\d+(?:\.\d+)?[kKmM]\s+tokens?\b|"
    r"\b(?:gpt|claude|gemini|grok|mistral|codestral|devstral|llama|o[1-9])-"
    r")",
    re.IGNORECASE,
)

FEATURE_TOKEN_RE = re.compile(r"\b[a-z][a-z0-9-]+-20\d{2}-\d{2}-\d{2}\b")
ISO_DATE_RE = re.compile(r"\b20\d{2}-\d{2}-\d{2}\b")
MODEL_TOKEN_RE = re.compile(
    r"\b(?:"
    r"(?:anthropic|deepseek|google|mistral|openai|x-ai)/[a-z0-9][a-z0-9._:/-]*|"
    r"(?:gpt|claude|gemini|grok|mistral|codestral|devstral|llama|o[1-9])"
    r"[a-z0-9._:/-]*"
    r")\b",
    re.IGNORECASE,
)


@dataclasses.dataclass(frozen=True)
class DocumentSource:
    provider: str
    name: str
    url: str
    kind: str = "document"  # document, llms, or sitemap
    discovery_patterns: tuple[str, ...] = ()


def load_provider_catalog(repo_root: Path, provider: str) -> dict[str, Any]:
    directory = CATALOG_DIRECTORIES[provider]
    path = repo_root / "providers" / directory / "catalog.json"
    try:
        catalog = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise FetchFailure(f"failed to read provider catalog {path}: {error}") from error
    expected = directory
    if catalog.get("schema_version") != SCHEMA_VERSION:
        raise FetchFailure(f"unsupported provider catalog schema in {path}")
    if catalog.get("provider") != expected:
        raise FetchFailure(
            f"provider catalog {path} declares {catalog.get('provider')!r}, expected {expected!r}"
        )
    return catalog


def catalog_document_sources(
    repo_root: Path, providers: Sequence[str]
) -> tuple[list[DocumentSource], dict[str, str]]:
    sources: list[DocumentSource] = []
    errors: dict[str, str] = {}
    for provider in providers:
        try:
            catalog = load_provider_catalog(repo_root, provider)
        except FetchFailure as error:
            errors[provider] = safe_error(error)
            continue
        for source in catalog.get("sources", []):
            kind = source.get("kind", "document")
            if kind == "api":
                continue
            sources.append(
                DocumentSource(
                    provider=provider,
                    name=source["name"],
                    url=source["url"],
                    kind=kind,
                    discovery_patterns=tuple(source.get("discovery_patterns", [])),
                )
            )
    return sources, errors


class FetchFailure(RuntimeError):
    """A source could not be fetched or decoded."""


class VisibleTextParser(HTMLParser):
    """Extract visible article text without requiring a third-party HTML parser."""

    BLOCK_TAGS = {
        "article",
        "blockquote",
        "br",
        "div",
        "dl",
        "dt",
        "dd",
        "figcaption",
        "figure",
        "h1",
        "h2",
        "h3",
        "h4",
        "h5",
        "h6",
        "li",
        "main",
        "p",
        "pre",
        "section",
        "table",
        "td",
        "th",
        "tr",
        "ul",
        "ol",
    }
    SKIP_TAGS = {"script", "style", "svg", "noscript", "template"}

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self._skip_depth = 0
        self._main_depth = 0
        self._saw_main = False
        self._all_parts: list[str] = []
        self._main_parts: list[str] = []

    def _append(self, value: str) -> None:
        self._all_parts.append(value)
        if self._main_depth:
            self._main_parts.append(value)

    def handle_starttag(
        self, tag: str, attrs: list[tuple[str, str | None]]
    ) -> None:
        tag = tag.lower()
        if tag in self.SKIP_TAGS:
            self._skip_depth += 1
            return
        if tag == "main":
            self._saw_main = True
            self._main_depth += 1
        if tag in self.BLOCK_TAGS and not self._skip_depth:
            self._append("\n")

    def handle_endtag(self, tag: str) -> None:
        tag = tag.lower()
        if tag in self.SKIP_TAGS:
            self._skip_depth = max(0, self._skip_depth - 1)
            return
        if tag in self.BLOCK_TAGS and not self._skip_depth:
            self._append("\n")
        if tag == "main" and self._main_depth:
            self._main_depth -= 1

    def handle_data(self, data: str) -> None:
        if not self._skip_depth:
            self._append(data)

    def text(self) -> str:
        parts = self._main_parts if self._saw_main and self._main_parts else self._all_parts
        return "".join(parts)


class HTTPClient:
    def __init__(
        self,
        timeout: float = DEFAULT_TIMEOUT_SECONDS,
        retries: int = DEFAULT_RETRIES,
    ) -> None:
        self.timeout = timeout
        self.retries = retries

    def request(
        self,
        url: str,
        *,
        method: str = "GET",
        headers: Mapping[str, str] | None = None,
        json_body: Any | None = None,
    ) -> tuple[bytes, str, str]:
        if urlparse(url).scheme.lower() not in {"http", "https"}:
            raise FetchFailure(f"unsupported URL scheme: {url}")
        request_headers = {
            "Accept": "application/json, text/markdown, text/plain, text/html, application/xml;q=0.9, */*;q=0.1",
            "User-Agent": USER_AGENT,
        }
        request_headers.update(headers or {})
        body: bytes | None = None
        if json_body is not None:
            body = json.dumps(json_body, separators=(",", ":")).encode("utf-8")
            request_headers["Content-Type"] = "application/json"

        last_error: Exception | None = None
        for attempt in range(self.retries + 1):
            try:
                request = Request(
                    url,
                    data=body,
                    headers=request_headers,
                    method=method,
                )
                with urlopen(request, timeout=self.timeout) as response:
                    data = response.read(MAX_RESPONSE_BYTES + 1)
                    if len(data) > MAX_RESPONSE_BYTES:
                        raise FetchFailure(
                            f"response exceeded {MAX_RESPONSE_BYTES} bytes: {url}"
                        )
                    content_type = response.headers.get_content_type()
                    return data, content_type, response.geturl()
            except FetchFailure:
                raise
            except HTTPError as error:
                last_error = error
                if error.code < 500 and error.code != 429:
                    break
            except (URLError, TimeoutError, OSError) as error:
                last_error = error
            if attempt < self.retries:
                time.sleep(0.5 * (2**attempt))
        raise FetchFailure(f"failed to fetch {url}: {last_error}")

    def json(
        self,
        url: str,
        *,
        method: str = "GET",
        headers: Mapping[str, str] | None = None,
        json_body: Any | None = None,
    ) -> Any:
        data, _, _ = self.request(
            url, method=method, headers=headers, json_body=json_body
        )
        try:
            return json.loads(data.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise FetchFailure(f"invalid JSON from {url}: {error}") from error


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def canonicalize(value: Any) -> Any:
    if isinstance(value, Mapping):
        return {
            str(key): canonicalize(item)
            for key, item in sorted(value.items(), key=lambda pair: str(pair[0]))
        }
    if isinstance(value, list):
        normalized = [canonicalize(item) for item in value]
        if all(isinstance(item, (str, int, float, bool, type(None))) for item in normalized):
            return sorted(normalized, key=lambda item: (str(type(item)), str(item)))
        return sorted(
            normalized,
            key=lambda item: json.dumps(item, sort_keys=True, separators=(",", ":")),
        )
    return value


def stable_json(value: Any, *, pretty: bool = False) -> str:
    if pretty:
        return json.dumps(canonicalize(value), indent=2, sort_keys=True) + "\n"
    return json.dumps(
        canonicalize(value), sort_keys=True, separators=(",", ":"), ensure_ascii=False
    )


def decode_text(data: bytes) -> str:
    return data.decode("utf-8", errors="replace")


def visible_text(data: bytes, content_type: str, url: str) -> str:
    text = decode_text(data)
    if "html" in content_type or urlparse(url).path.endswith((".html", "/")):
        parser = VisibleTextParser()
        parser.feed(text)
        text = parser.text()
    return unicodedata.normalize("NFKC", html.unescape(text))


def normalize_lines(text: str) -> list[str]:
    lines: list[str] = []
    for raw_line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n"):
        line = re.sub(r"\s+", " ", raw_line).strip()
        if not line:
            continue
        if len(line) > 600:
            line = line[:597] + "..."
        lines.append(line)
    return lines


def extract_markdown_links(text: str, base_url: str) -> list[str]:
    links = re.findall(r"\[[^\]]*\]\(([^)\s]+)", text)
    return sorted(
        {
            urljoin(base_url, link.removesuffix(".md"))
            for link in links
            if not link.startswith(("#", "mailto:"))
        }
    )


def extract_sitemap_links(data: bytes) -> list[str]:
    try:
        root = ET.fromstring(data)
    except ET.ParseError as error:
        raise FetchFailure(f"invalid sitemap XML: {error}") from error
    links: set[str] = set()
    for element in root.iter():
        if element.tag.endswith("loc") and element.text:
            links.add(element.text.strip())
    return sorted(links)


def filter_discovery_links(
    links: Iterable[str], patterns: Sequence[str]
) -> list[str]:
    compiled = [re.compile(pattern, re.IGNORECASE) for pattern in patterns]
    return sorted({link for link in links if any(p.search(link) for p in compiled)})


def document_signals(lines: Iterable[str]) -> list[str]:
    return sorted({line for line in lines if MATERIAL_LINE_RE.search(line)})


def collect_sitemap_links(
    client: HTTPClient,
    data: bytes,
    *,
    max_child_sitemaps: int = 20,
) -> list[str]:
    """Expand one level of sitemap indexes while keeping network work bounded."""

    links = extract_sitemap_links(data)
    child_sitemaps = [
        link
        for link in links
        if urlparse(link).path.lower().endswith((".xml", ".xml.gz"))
    ]
    if not child_sitemaps:
        return links

    expanded: set[str] = set(links)
    for child_url in child_sitemaps[:max_child_sitemaps]:
        try:
            child_data, _, _ = client.request(child_url)
            if urlparse(child_url).path.lower().endswith(".xml.gz"):
                child_data = gzip.decompress(child_data)
            expanded.update(extract_sitemap_links(child_data))
        except (FetchFailure, OSError, EOFError):
            # The index itself remains useful if one partition is unavailable.
            continue
    return sorted(expanded)


def collect_document(client: HTTPClient, source: DocumentSource) -> dict[str, Any]:
    accept = {
        "llms": "text/plain",
        "sitemap": "application/xml, text/xml;q=0.9, */*;q=0.1",
    }.get(source.kind, "text/markdown, text/html;q=0.9, text/plain;q=0.8")
    data, content_type, effective_url = client.request(
        source.url,
        headers={"Accept": accept},
    )
    raw_hash = hashlib.sha256(data).hexdigest()
    decoded = decode_text(data)

    if source.kind == "sitemap":
        links = filter_discovery_links(
            collect_sitemap_links(client, data), source.discovery_patterns
        )
        signals = links
    else:
        text = visible_text(data, content_type, effective_url)
        lines = normalize_lines(text)
        signals = document_signals(lines)
        links: list[str] = []
        if source.kind == "llms":
            links = filter_discovery_links(
                extract_markdown_links(decoded, effective_url),
                source.discovery_patterns,
            )
            signals = sorted(set(signals).union(links))

    semantic_text = "\n".join(signals)
    return {
        "status": "ok",
        "url": source.url,
        "effective_url": effective_url,
        "content_type": content_type,
        "raw_sha256": raw_hash,
        "semantic_sha256": sha256_text(semantic_text),
        "signals": signals,
        "model_tokens": sorted(set(MODEL_TOKEN_RE.findall(semantic_text))),
        "feature_tokens": sorted(set(FEATURE_TOKEN_RE.findall(semantic_text))),
        "dates": sorted(set(ISO_DATE_RE.findall(semantic_text))),
    }


def query_url(url: str, params: Mapping[str, Any]) -> str:
    return f"{url}?{urlencode(params)}"


def select_fields(record: Mapping[str, Any], fields: Sequence[str]) -> dict[str, Any]:
    return {field: record[field] for field in fields if field in record}


def records_by_id(
    records: Iterable[Mapping[str, Any]],
    fields: Sequence[str],
    *,
    key: Callable[[Mapping[str, Any]], str] | None = None,
    include: Callable[[Mapping[str, Any]], bool] | None = None,
) -> dict[str, Any]:
    output: dict[str, Any] = {}
    for record in records:
        if include and not include(record):
            continue
        model_id = key(record) if key else str(record.get("id") or record.get("name") or "")
        if not model_id:
            continue
        output[model_id] = canonicalize(select_fields(record, fields))
    return dict(sorted(output.items()))


def collect_openai_api(client: HTTPClient, key: str) -> dict[str, Any]:
    url = "https://api.openai.com/v1/models"
    payload = client.json(url, headers={"Authorization": f"Bearer {key}"})
    models = records_by_id(
        payload.get("data", []),
        ("id", "created", "owned_by", "object"),
        include=lambda item: not str(item.get("id", "")).startswith("ft:"),
    )
    return {"status": "ok", "source": url, "models": models}


def collect_anthropic_api(client: HTTPClient, key: str) -> dict[str, Any]:
    base_url = "https://api.anthropic.com/v1/models"
    headers = {
        "anthropic-version": "2023-06-01",
        "x-api-key": key,
    }
    records: list[Mapping[str, Any]] = []
    after_id: str | None = None
    for _ in range(20):
        params: dict[str, Any] = {"limit": 1000}
        if after_id:
            params["after_id"] = after_id
        payload = client.json(query_url(base_url, params), headers=headers)
        records.extend(payload.get("data", []))
        if not payload.get("has_more"):
            break
        after_id = payload.get("last_id")
        if not after_id:
            break
    fields = (
        "id",
        "type",
        "display_name",
        "created_at",
        "max_input_tokens",
        "max_output_tokens",
        "betas",
        "capabilities",
    )
    models = records_by_id(
        records,
        fields,
        include=lambda item: not str(item.get("id", "")).startswith("ft:"),
    )
    return {"status": "ok", "source": base_url, "models": models}


def collect_google_api(client: HTTPClient, key: str) -> dict[str, Any]:
    base_url = "https://generativelanguage.googleapis.com/v1beta/models"
    records: list[Mapping[str, Any]] = []
    page_token: str | None = None
    for _ in range(20):
        params: dict[str, Any] = {"key": key, "pageSize": 1000}
        if page_token:
            params["pageToken"] = page_token
        payload = client.json(query_url(base_url, params))
        records.extend(payload.get("models", []))
        page_token = payload.get("nextPageToken")
        if not page_token:
            break
    fields = (
        "name",
        "baseModelId",
        "version",
        "displayName",
        "description",
        "inputTokenLimit",
        "outputTokenLimit",
        "supportedGenerationMethods",
        "thinking",
        "temperature",
        "maxTemperature",
        "topP",
        "topK",
    )
    models = records_by_id(
        records,
        fields,
        key=lambda item: str(item.get("name", "")).removeprefix("models/"),
    )
    return {"status": "ok", "source": base_url, "models": models}


def collect_xai_api(client: HTTPClient, key: str) -> dict[str, Any]:
    base_url = "https://api.x.ai/v1"
    headers = {"Authorization": f"Bearer {key}"}
    fields = (
        "id",
        "aliases",
        "context_length",
        "max_prompt_length",
        "fingerprint",
        "version",
        "created",
        "input_modalities",
        "output_modalities",
        "prompt_text_token_price",
        "cached_prompt_text_token_price",
        "prompt_image_token_price",
        "completion_text_token_price",
        "generated_image_token_price",
        "image_price",
        "search_price",
        "prompt_text_token_price_long_context",
        "cached_prompt_text_token_price_long_context",
        "completion_text_token_price_long_context",
        "long_context_threshold",
    )
    models: dict[str, Any] = {}
    endpoints = {
        "language": "language-models",
        "image": "image-generation-models",
        "video": "video-generation-models",
    }
    for kind, endpoint in endpoints.items():
        url = f"{base_url}/{endpoint}"
        payload = client.json(url, headers=headers)
        records = payload.get("models", payload.get("data", []))
        models.update(
            records_by_id(
                records,
                fields,
                key=lambda item, prefix=kind: (
                    f"{prefix}:{item['id']}" if item.get("id") else ""
                ),
            )
        )
    return {"status": "ok", "source": base_url, "models": dict(sorted(models.items()))}


def collect_mistral_api(client: HTTPClient, key: str) -> dict[str, Any]:
    url = "https://api.mistral.ai/v1/models"
    payload = client.json(url, headers={"Authorization": f"Bearer {key}"})
    records = payload.get("data", payload if isinstance(payload, list) else [])
    fields = (
        "id",
        "object",
        "created",
        "owned_by",
        "root",
        "type",
        "TYPE",
        "max_context_length",
        "aliases",
        "capabilities",
        "archived",
    )

    def public_model(item: Mapping[str, Any]) -> bool:
        model_id = str(item.get("id", ""))
        kind = str(item.get("type") or item.get("TYPE") or "").lower()
        return not model_id.startswith("ft:") and "fine-tuned" not in kind

    models = records_by_id(records, fields, include=public_model)
    return {"status": "ok", "source": url, "models": models}


def collect_openrouter_api(client: HTTPClient, key: str | None) -> dict[str, Any]:
    url = "https://openrouter.ai/api/v1/models?output_modalities=all"
    headers = {"Authorization": f"Bearer {key}"} if key else {}
    payload = client.json(url, headers=headers)
    fields = (
        "id",
        "canonical_slug",
        "name",
        "created",
        "context_length",
        "expiration_date",
        "knowledge_cutoff",
        "architecture",
        "pricing",
        "top_provider",
        "per_request_limits",
        "supported_parameters",
        "supported_voices",
        "default_parameters",
    )
    models = records_by_id(
        payload.get("data", []),
        fields,
        include=lambda item: str(item.get("id", "")).startswith(OPENROUTER_PREFIXES),
    )
    return {"status": "ok", "source": url, "models": models}


def collect_ollama_api(client: HTTPClient, base_url: str) -> dict[str, Any]:
    tags_url = f"{base_url.rstrip('/')}/api/tags"
    payload = client.json(tags_url)
    records = payload.get("models", [])
    models = records_by_id(
        records,
        ("name", "model", "modified_at", "digest", "size", "details"),
        key=lambda item: str(item.get("model") or item.get("name") or ""),
    )
    for model_id, model in list(models.items())[:20]:
        try:
            show = client.json(
                f"{base_url.rstrip('/')}/api/show",
                method="POST",
                json_body={"model": model_id},
            )
            model["capabilities"] = show.get("capabilities", [])
            if "model_info" in show:
                model["model_info"] = show["model_info"]
        except FetchFailure as error:
            model["show_error"] = str(error)
    return {"status": "ok", "source": tags_url, "models": models}


def collect_ollama_releases(client: HTTPClient) -> dict[str, Any]:
    url = "https://api.github.com/repos/ollama/ollama/releases?per_page=20"
    payload = client.json(url, headers={"Accept": "application/vnd.github+json"})
    releases = [
        select_fields(item, ("tag_name", "name", "published_at", "html_url", "prerelease"))
        for item in payload
    ]
    return {"status": "ok", "source": url, "releases": canonicalize(releases)}


API_COLLECTORS: dict[str, tuple[tuple[str, ...], Callable[[HTTPClient, str], dict[str, Any]]]] = {
    "openai": (("OPENAI_API_KEY",), collect_openai_api),
    "anthropic": (("ANTHROPIC_API_KEY",), collect_anthropic_api),
    "google": (("GEMINI_API_KEY", "GOOGLE_API_KEY"), collect_google_api),
    "xai": (("XAI_API_KEY", "GROK_API_KEY"), collect_xai_api),
    "mistral": (("MISTRAL_API_KEY",), collect_mistral_api),
}


def first_env(names: Sequence[str]) -> str | None:
    for name in names:
        value = os.getenv(name)
        if value:
            return value
    return None


def safe_error(error: Exception) -> str:
    message = str(error)
    for env_name in (
        "OPENAI_API_KEY",
        "ANTHROPIC_API_KEY",
        "GEMINI_API_KEY",
        "GOOGLE_API_KEY",
        "XAI_API_KEY",
        "GROK_API_KEY",
        "MISTRAL_API_KEY",
        "OPENROUTER_API_KEY",
    ):
        value = os.getenv(env_name)
        if value:
            message = message.replace(value, "[redacted]")
    return message


def collect_repo_facts(repo_root: Path) -> dict[str, Any]:
    facts: dict[str, Any] = {"providers": {}}
    cli_model_ids: set[str] = set()
    for provider in PROVIDERS:
        try:
            catalog = load_provider_catalog(repo_root, provider)
        except FetchFailure:
            continue

        models_by_name = {
            model["go_name"]: model for model in catalog.get("models", [])
        }

        def model_id(
            model: Mapping[str, Any],
            models_by_name: Mapping[str, Any] = models_by_name,
        ) -> str | None:
            if model.get("id"):
                return str(model["id"])
            target = models_by_name.get(str(model.get("alias_of", "")))
            return str(target["id"]) if target and target.get("id") else None

        models = sorted(
            {
                resolved
                for model in catalog.get("models", [])
                if (resolved := model_id(model))
            }
        )
        defaults = sorted(
            {
                resolved
                for model in catalog.get("models", [])
                if model.get("default") and (resolved := model_id(model))
            }
        )
        pricing = {
            entry["model"]: canonicalize(entry)
            for entry in (catalog.get("pricing", {}) or {}).get("text", [])
        }
        facts["providers"][provider] = {
            "models": models,
            "defaults": defaults,
            "pricing": dict(sorted(pricing.items())),
            "feature_flags": sorted(
                feature["id"]
                for feature in catalog.get("feature_flags", [])
                if feature.get("id")
            ),
        }
        cli_model_ids.update(
            resolved
            for model in catalog.get("models", [])
            if model.get("recommended") and (resolved := model_id(model))
        )

    facts["cli_model_ids"] = sorted(cli_model_ids)
    return canonicalize(facts)


def collect_sdk_versions(repo_root: Path) -> dict[str, Any]:
    modules = (
        (repo_root / "providers" / "openai", "github.com/openai/openai-go/v3"),
        (repo_root / "providers" / "google", "google.golang.org/genai"),
    )
    output: dict[str, Any] = {}
    for workdir, module in modules:
        if not (workdir / "go.mod").exists():
            continue
        try:
            process = subprocess.run(
                ["go", "list", "-m", "-u", "-json", module],
                cwd=workdir,
                check=True,
                capture_output=True,
                text=True,
                timeout=60,
            )
            payload = json.loads(process.stdout)
            output[module] = canonicalize(
                {
                    "version": payload.get("Version"),
                    "update": (payload.get("Update") or {}).get("Version"),
                }
            )
        except (OSError, subprocess.SubprocessError, json.JSONDecodeError) as error:
            output[module] = {"error": safe_error(error)}
    return output


def empty_provider() -> dict[str, Any]:
    return {
        "api": {"status": "skipped", "reason": "not configured", "models": {}},
        "documents": {},
        "metadata": {},
    }


def build_snapshot(
    *,
    client: HTTPClient,
    providers: Sequence[str],
    public_only: bool,
    include_local_ollama: bool,
    ollama_url: str,
    repo_root: Path,
    check_sdks: bool,
) -> dict[str, Any]:
    result: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "generated_at": utc_now(),
        "providers": {provider: empty_provider() for provider in providers},
        "repo": collect_repo_facts(repo_root),
    }

    sources, catalog_errors = catalog_document_sources(repo_root, providers)
    if catalog_errors:
        result["errors"] = catalog_errors
    with ThreadPoolExecutor(max_workers=min(8, len(sources) or 1)) as executor:
        pending = {
            executor.submit(collect_document, client, source): source
            for source in sources
        }
        for future in as_completed(pending):
            source = pending[future]
            try:
                document = future.result()
            except FetchFailure as error:
                document = {
                    "status": "error",
                    "url": source.url,
                    "error": safe_error(error),
                }
            result["providers"][source.provider]["documents"][source.name] = document

    for provider in providers:
        provider_result = result["providers"][provider]
        try:
            if provider == "openrouter":
                provider_result["api"] = collect_openrouter_api(
                    client, first_env(("OPENROUTER_API_KEY",))
                )
            elif provider == "ollama":
                try:
                    provider_result["metadata"]["releases"] = collect_ollama_releases(
                        client
                    )
                except FetchFailure as error:
                    provider_result["metadata"]["releases"] = {
                        "status": "error",
                        "source": "https://api.github.com/repos/ollama/ollama/releases",
                        "error": safe_error(error),
                    }
                if include_local_ollama:
                    provider_result["api"] = collect_ollama_api(client, ollama_url)
                else:
                    provider_result["api"] = {
                        "status": "skipped",
                        "reason": "local Ollama disabled",
                        "models": {},
                    }
            elif not public_only and provider in API_COLLECTORS:
                env_names, collector = API_COLLECTORS[provider]
                key = first_env(env_names)
                if key:
                    provider_result["api"] = collector(client, key)
                else:
                    provider_result["api"] = {
                        "status": "skipped",
                        "reason": f"none of {', '.join(env_names)} is set",
                        "models": {},
                    }
        except FetchFailure as error:
            provider_result["api"] = {
                "status": "error",
                "error": safe_error(error),
                "models": {},
            }

    if check_sdks:
        result["sdks"] = collect_sdk_versions(repo_root)
    return canonicalize(result)


def recursive_field_diff(before: Any, after: Any, prefix: str = "") -> list[dict[str, Any]]:
    if before == after:
        return []
    if isinstance(before, Mapping) and isinstance(after, Mapping):
        changes: list[dict[str, Any]] = []
        for key in sorted(set(before).union(after)):
            path = f"{prefix}.{key}" if prefix else str(key)
            if key not in before:
                changes.append({"field": path, "before": None, "after": after[key]})
            elif key not in after:
                changes.append({"field": path, "before": before[key], "after": None})
            else:
                changes.extend(recursive_field_diff(before[key], after[key], path))
        return changes
    return [{"field": prefix or "value", "before": before, "after": after}]


def classify_document_change(added: Sequence[str], removed: Sequence[str]) -> str:
    joined = "\n".join((*added, *removed)).lower()
    if any(word in joined for word in ("deprecated", "retired", "shutdown", "replacement")):
        return "lifecycle_changed"
    if "$" in joined or "pricing" in joined or "price" in joined:
        return "pricing_changed"
    if FEATURE_TOKEN_RE.search(joined):
        return "feature_flag_changed"
    if any(
        word in joined
        for word in (
            "reasoning",
            "thinking",
            "temperature",
            "structured output",
            "function calling",
            "beta",
            "context",
        )
    ):
        return "capability_changed"
    if MODEL_TOKEN_RE.search(joined):
        return "model_catalog_changed"
    return "document_changed"


def classify_model_change(fields: Sequence[Mapping[str, Any]]) -> str:
    paths = "\n".join(str(field.get("field", "")).lower() for field in fields)
    if any(word in paths for word in ("pricing", "price", "cost")):
        return "pricing_changed"
    if any(
        word in paths
        for word in ("deprecated", "archived", "expiration", "retired")
    ):
        return "lifecycle_changed"
    if any(
        word in paths
        for word in (
            "capabilit",
            "context",
            "modality",
            "modalities",
            "parameter",
            "reasoning",
            "thinking",
            "temperature",
            "token",
            "top_",
            "topprovider",
        )
    ):
        return "capability_changed"
    return "model_changed"


def dive_model_status(
    snapshot: Mapping[str, Any], provider: str, upstream_model_id: str
) -> dict[str, bool]:
    model_id = upstream_model_id
    if provider == "xai" and ":" in model_id:
        model_id = model_id.split(":", 1)[1]
    repo = snapshot.get("repo", {}) or {}
    provider_facts = (repo.get("providers", {}) or {}).get(provider, {})
    return {
        "provider_catalog": model_id in provider_facts.get("models", []),
        "cli_recommended": model_id in repo.get("cli_model_ids", []),
    }


def diff_snapshots(before: Mapping[str, Any], after: Mapping[str, Any]) -> dict[str, Any]:
    changes: list[dict[str, Any]] = []
    providers = sorted(
        set((before.get("providers") or {}).keys()).union(
            (after.get("providers") or {}).keys()
        )
    )
    for provider in providers:
        old_provider = (before.get("providers") or {}).get(provider, {})
        new_provider = (after.get("providers") or {}).get(provider, {})

        old_api = old_provider.get("api", {})
        new_api = new_provider.get("api", {})
        if old_api.get("status") == "ok" and new_api.get("status") != "ok":
            changes.append(
                {
                    "provider": provider,
                    "kind": "source_error",
                    "source": "api",
                    "before": "ok",
                    "after": new_api.get("status"),
                    "details": new_api.get("error") or new_api.get("reason"),
                }
            )
        if old_api.get("status") == "ok" and new_api.get("status") == "ok":
            old_models = old_api.get("models", {})
            new_models = new_api.get("models", {})
            for model_id in sorted(set(new_models) - set(old_models)):
                changes.append(
                    {
                        "provider": provider,
                        "kind": "model_added",
                        "model": model_id,
                        "after": new_models[model_id],
                        "dive": dive_model_status(after, provider, model_id),
                    }
                )
            for model_id in sorted(set(old_models) - set(new_models)):
                changes.append(
                    {
                        "provider": provider,
                        "kind": "model_removed",
                        "model": model_id,
                        "before": old_models[model_id],
                        "dive": dive_model_status(after, provider, model_id),
                    }
                )
            for model_id in sorted(set(old_models).intersection(new_models)):
                fields = recursive_field_diff(old_models[model_id], new_models[model_id])
                if fields:
                    changes.append(
                        {
                            "provider": provider,
                            "kind": classify_model_change(fields),
                            "model": model_id,
                            "fields": fields,
                        }
                    )

        old_documents = old_provider.get("documents", {})
        new_documents = new_provider.get("documents", {})
        for name in sorted(set(old_documents).union(new_documents)):
            old_doc = old_documents.get(name)
            new_doc = new_documents.get(name)
            if old_doc is None:
                changes.append(
                    {
                        "provider": provider,
                        "kind": "document_source_added",
                        "document": name,
                        "url": (new_doc or {}).get("url"),
                    }
                )
                continue
            if new_doc is None:
                changes.append(
                    {
                        "provider": provider,
                        "kind": "document_source_removed",
                        "document": name,
                        "url": old_doc.get("url"),
                    }
                )
                continue
            if old_doc.get("status") == "ok" and new_doc.get("status") != "ok":
                changes.append(
                    {
                        "provider": provider,
                        "kind": "source_error",
                        "source": f"document:{name}",
                        "url": new_doc.get("url"),
                        "details": new_doc.get("error"),
                    }
                )
                continue
            if old_doc.get("status") != "ok" or new_doc.get("status") != "ok":
                continue
            if old_doc.get("semantic_sha256") == new_doc.get("semantic_sha256"):
                continue
            old_signals = set(old_doc.get("signals", []))
            new_signals = set(new_doc.get("signals", []))
            added = sorted(new_signals - old_signals)
            removed = sorted(old_signals - new_signals)
            added_features = sorted(
                set(new_doc.get("feature_tokens", []))
                - set(old_doc.get("feature_tokens", []))
            )
            supported_features = set(
                ((after.get("repo", {}) or {}).get("providers", {}) or {})
                .get(provider, {})
                .get("feature_flags", [])
            )
            change = {
                "provider": provider,
                "kind": classify_document_change(added, removed),
                "document": name,
                "url": new_doc.get("url"),
                "added": added,
                "removed": removed,
            }
            if added_features:
                change["feature_flags_added"] = added_features
                change["unlisted_feature_flags"] = sorted(
                    set(added_features) - supported_features
                )
            changes.append(change)

        old_metadata = old_provider.get("metadata", {})
        new_metadata = new_provider.get("metadata", {})
        for name in sorted(set(old_metadata).union(new_metadata)):
            old_item = old_metadata.get(name)
            new_item = new_metadata.get(name)
            if old_item is None or new_item is None:
                changes.append(
                    {
                        "provider": provider,
                        "kind": "metadata_source_changed",
                        "source": f"metadata:{name}",
                        "before": old_item,
                        "after": new_item,
                    }
                )
                continue
            if old_item.get("status") == "ok" and new_item.get("status") != "ok":
                changes.append(
                    {
                        "provider": provider,
                        "kind": "source_error",
                        "source": f"metadata:{name}",
                        "details": new_item.get("error"),
                    }
                )
                continue
            if old_item.get("status") != "ok" or new_item.get("status") != "ok":
                continue
            fields = recursive_field_diff(old_item, new_item)
            if fields:
                changes.append(
                    {
                        "provider": provider,
                        "kind": (
                            "release_changed"
                            if name == "releases"
                            else "metadata_changed"
                        ),
                        "source": f"metadata:{name}",
                        "fields": fields,
                    }
                )

    before_sdks = before.get("sdks") or {}
    after_sdks = after.get("sdks") or {}
    for module in sorted(set(before_sdks).intersection(after_sdks)):
        if before_sdks[module] != after_sdks[module]:
            changes.append(
                {
                    "provider": "sdk",
                    "kind": "sdk_changed",
                    "module": module,
                    "fields": recursive_field_diff(
                        before_sdks[module], after_sdks[module]
                    ),
                }
            )

    material = canonicalize(changes)
    change_hash = sha256_text(stable_json(material)) if material else None
    return {
        "schema_version": SCHEMA_VERSION,
        "generated_at": after.get("generated_at") or utc_now(),
        "has_changes": bool(material),
        "change_count": len(material),
        "change_hash": change_hash,
        "changes": material,
    }


def summarize_value(value: Any, limit: int = 180) -> str:
    rendered = stable_json(value)
    if len(rendered) > limit:
        return rendered[: limit - 3] + "..."
    return rendered


def escape_markdown(value: Any) -> str:
    """Render provider-controlled text without allowing Markdown or HTML markup."""

    text = str(value).replace("\r", " ").replace("\n", " ")
    return text.translate(
        str.maketrans(
            {
                "\\": "&#92;",
                "`": "&#96;",
                "*": "&#42;",
                "_": "&#95;",
                "{": "&#123;",
                "}": "&#125;",
                "[": "&#91;",
                "]": "&#93;",
                "(": "&#40;",
                ")": "&#41;",
                "<": "&lt;",
                ">": "&gt;",
                "#": "&#35;",
                "+": "&#43;",
                "-": "&#45;",
                ".": "&#46;",
                "!": "&#33;",
                "|": "&#124;",
                "&": "&amp;",
            }
        )
    )


def report_markdown(diff: Mapping[str, Any], current: Mapping[str, Any]) -> str:
    changes = diff.get("changes", [])
    displayed_changes = changes[:MAX_REPORT_CHANGES]
    lines = [
        "# Dive provider watch report",
        "",
        f"Generated: `{diff.get('generated_at')}`",
        "",
    ]
    if not changes:
        lines.extend(("No material upstream changes detected.", ""))
        return "\n".join(lines)

    lines.extend(
        (
            f"Detected **{len(changes)}** material change(s).",
            "",
            f"Change hash: `{diff.get('change_hash')}`",
            "",
        )
    )
    if len(changes) > len(displayed_changes):
        lines.extend(
            (
                f"Showing the first {len(displayed_changes)} changes; "
                "the JSON artifact contains the complete diff.",
                "",
            )
        )
    lines.extend(
        (
            "| Provider | Change | Subject | Dive catalog |",
            "| --- | --- | --- | --- |",
        )
    )
    for change in displayed_changes:
        subject = (
            change.get("model")
            or change.get("document")
            or change.get("module")
            or change.get("source")
            or "-"
        )
        dive = change.get("dive")
        dive_status = (
            "listed" if dive and dive.get("provider_catalog") else "not listed"
        ) if dive is not None else "-"
        lines.append(
            f"| {change.get('provider')} | `{change.get('kind')}` | "
            f"<code>{escape_markdown(subject)}</code> | "
            f"{dive_status} |"
        )

    for change in displayed_changes:
        subject = (
            change.get("model")
            or change.get("document")
            or change.get("module")
            or change.get("source")
            or "change"
        )
        lines.extend(
            ("", f"## {change.get('provider')}: {escape_markdown(subject)}", "")
        )
        lines.append(f"Type: `{change.get('kind')}`")
        if change.get("url"):
            lines.extend(("", f"Source: {change['url']}"))
        if change.get("details"):
            lines.extend(("", f"Details: {escape_markdown(change['details'])}"))
        if change.get("dive") is not None:
            dive = change["dive"]
            lines.extend(
                (
                    "",
                    "Dive provider catalog: "
                    + ("listed" if dive.get("provider_catalog") else "not listed"),
                    "Dive CLI recommendations: "
                    + ("listed" if dive.get("cli_recommended") else "not listed"),
                )
            )
        if change.get("before") is not None:
            lines.extend(
                (
                    "",
                    f"Before: <code>{escape_markdown(summarize_value(change['before']))}</code>",
                )
            )
        if change.get("after") is not None:
            lines.extend(
                (
                    "",
                    f"After: <code>{escape_markdown(summarize_value(change['after']))}</code>",
                )
            )
        fields = change.get("fields", [])
        if fields:
            lines.extend(("", "Changed fields:", ""))
            for field in fields[:30]:
                lines.append(
                    f"- <code>{escape_markdown(field['field'])}</code>: "
                    f"<code>{escape_markdown(summarize_value(field.get('before')))}</code> → "
                    f"<code>{escape_markdown(summarize_value(field.get('after')))}</code>"
                )
            if len(fields) > 30:
                lines.append(f"- … {len(fields) - 30} more field changes")
        for label, key in (("Added signals", "added"), ("Removed signals", "removed")):
            values = change.get(key, [])
            if not values:
                continue
            lines.extend(("", f"{label}:", ""))
            for value in values[:25]:
                lines.append(f"- {escape_markdown(value)}")
            if len(values) > 25:
                lines.append(f"- … {len(values) - 25} more")
        if change.get("feature_flags_added"):
            lines.extend(("", "New upstream feature flags:", ""))
            unlisted = set(change.get("unlisted_feature_flags", []))
            for feature in change["feature_flags_added"]:
                status = "not listed in Dive" if feature in unlisted else "listed in Dive"
                lines.append(f"- <code>{escape_markdown(feature)}</code> — {status}")

    repo_models = {
        provider: len(data.get("models", []))
        for provider, data in (current.get("repo", {}).get("providers", {}) or {}).items()
    }
    if repo_models:
        lines.extend(("", "## Dive context", ""))
        lines.append(
            "Current model constants found in the repository: "
            + ", ".join(f"{provider}={count}" for provider, count in sorted(repo_models.items()))
            + "."
        )
    lines.append("")
    return "\n".join(lines)


def load_json(path: Path) -> Any:
    try:
        if path.suffix == ".gz":
            content = gzip.decompress(path.read_bytes()).decode("utf-8")
        else:
            content = path.read_text(encoding="utf-8")
        return json.loads(content)
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise SystemExit(f"failed to read {path}: {error}") from error


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def write_json(path: Path, value: Any) -> None:
    content = stable_json(value, pretty=True)
    if path.suffix == ".gz":
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(gzip.compress(content.encode("utf-8"), mtime=0))
    else:
        write_text(path, content)


def write_github_output(path: Path, diff: Mapping[str, Any], report_path: Path) -> None:
    values = {
        "has_changes": str(bool(diff.get("has_changes"))).lower(),
        "change_count": str(diff.get("change_count", 0)),
        "change_hash": str(diff.get("change_hash") or ""),
        "report_path": str(report_path),
    }
    with path.open("a", encoding="utf-8") as output:
        for key, value in values.items():
            output.write(f"{key}={value}\n")


def add_common_collection_flags(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--provider",
        dest="providers",
        action="append",
        choices=PROVIDERS,
        help="provider to collect; repeat to select multiple (default: all)",
    )
    parser.add_argument(
        "--public-only",
        action="store_true",
        help="skip account-authenticated provider APIs; OpenRouter remains enabled",
    )
    parser.add_argument(
        "--include-local-ollama",
        action="store_true",
        help="query the Ollama instance in --ollama-url",
    )
    parser.add_argument(
        "--ollama-url",
        default=os.getenv("OLLAMA_HOST", "http://localhost:11434"),
    )
    parser.add_argument("--timeout", type=float, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--retries", type=int, default=DEFAULT_RETRIES)
    parser.add_argument(
        "--repo-root", type=Path, default=Path(__file__).resolve().parents[1]
    )
    parser.add_argument(
        "--check-sdks",
        action="store_true",
        help="run go list against the OpenAI and Google provider modules",
    )


def collect_from_args(args: argparse.Namespace) -> dict[str, Any]:
    client = HTTPClient(timeout=args.timeout, retries=args.retries)
    return build_snapshot(
        client=client,
        providers=args.providers or list(PROVIDERS),
        public_only=args.public_only,
        include_local_ollama=args.include_local_ollama,
        ollama_url=args.ollama_url,
        repo_root=args.repo_root.resolve(),
        check_sdks=args.check_sdks,
    )


def command_snapshot(args: argparse.Namespace) -> int:
    snapshot = collect_from_args(args)
    write_json(args.output, snapshot)
    print(f"wrote provider snapshot to {args.output}")
    return 0


def finish_audit(
    *,
    before: Mapping[str, Any],
    current: Mapping[str, Any],
    snapshot_path: Path | None,
    report_path: Path,
    json_report_path: Path | None,
    github_output: Path | None,
    fail_on_change: bool,
) -> int:
    diff = diff_snapshots(before, current)
    if snapshot_path:
        write_json(snapshot_path, current)
    markdown = report_markdown(diff, current)
    write_text(report_path, markdown)
    if json_report_path:
        write_json(json_report_path, diff)
    if github_output:
        write_github_output(github_output, diff, report_path)
    print(markdown)
    if fail_on_change and diff["has_changes"]:
        return 2
    return 0


def command_audit(args: argparse.Namespace) -> int:
    baseline = load_json(args.baseline)
    current = collect_from_args(args)
    return finish_audit(
        before=baseline,
        current=current,
        snapshot_path=args.snapshot,
        report_path=args.report,
        json_report_path=args.json_report,
        github_output=args.github_output,
        fail_on_change=args.fail_on_change,
    )


def command_diff(args: argparse.Namespace) -> int:
    before = load_json(args.baseline)
    current = load_json(args.current)
    return finish_audit(
        before=before,
        current=current,
        snapshot_path=None,
        report_path=args.report,
        json_report_path=args.json_report,
        github_output=args.github_output,
        fail_on_change=args.fail_on_change,
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    snapshot = subparsers.add_parser("snapshot", help="collect and save a snapshot")
    add_common_collection_flags(snapshot)
    snapshot.add_argument("--output", type=Path, required=True)
    snapshot.set_defaults(func=command_snapshot)

    audit = subparsers.add_parser(
        "audit", help="collect a snapshot and compare it with a baseline"
    )
    add_common_collection_flags(audit)
    audit.add_argument("--baseline", type=Path, required=True)
    audit.add_argument("--snapshot", type=Path)
    audit.add_argument("--report", type=Path, required=True)
    audit.add_argument("--json-report", type=Path)
    audit.add_argument("--github-output", type=Path)
    audit.add_argument("--fail-on-change", action="store_true")
    audit.set_defaults(func=command_audit)

    diff = subparsers.add_parser(
        "diff", help="compare two snapshots without network access"
    )
    diff.add_argument("--baseline", type=Path, required=True)
    diff.add_argument("--current", type=Path, required=True)
    diff.add_argument("--report", type=Path, required=True)
    diff.add_argument("--json-report", type=Path)
    diff.add_argument("--github-output", type=Path)
    diff.add_argument("--fail-on-change", action="store_true")
    diff.set_defaults(func=command_diff)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main())
