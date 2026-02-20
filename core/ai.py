"""
core/ai.py
~~~~~~~~~~
AI-powered email analysis using LiteLLM for multi-provider LLM routing.

Responsibilities:
- Build dynamic prompts from JSON config + Markdown rules + task context
- Analyze email content and produce structured output (summary, category, translation)
- Support multiple LLM providers via litellm
- Smart translation when source language != target language
- Graceful fallback on API errors
"""

from __future__ import annotations

import json
import logging
import re
import socket
import sys
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

from core.models import AIAnalysisResult, AIConfig

logger = logging.getLogger("mailbot.ai")

# ──────────────────────────────────────────────
#  Language mapping
# ──────────────────────────────────────────────

LANGUAGE_NAMES: dict[str, str] = {
    "zh": "Chinese",
    "en": "English",
    "ja": "Japanese",
    "auto": "the same language as the email",
}

# ──────────────────────────────────────────────
#  Base system prompt (lowest priority, overridden by rules & config)
# ──────────────────────────────────────────────

BASE_SYSTEM_PROMPT = """You are a professional mail triage assistant. Analyze the email content and return a JSON object that strictly matches the provided schema.

Requirements:
1. summary: concise summary in 50-100 words, focusing on the main intent
2. category: choose one of [verification_code, notification, billing, promotion, personal]
3. priority: integer 1-5 (1=urgent, 5=low)
    - verification_code / security → 1
    - personal / important notification → 2
    - billing / subscription → 3
    - general notification → 4
    - promotion / marketing → 5
4. extracted_code: if the email contains a verification/pickup/auth code, extract it; otherwise use null
5. source_language: detect the primary language of this email (use ISO 639-1 code, e.g. "en", "zh", "ja", "fr")
6. translation: see the Translation Rule below

Return only a single JSON object without extra text or markdown."""

LANGUAGE_CONSTRAINT_TEMPLATE = """[System Constraint — HIGHEST PRIORITY]
You MUST write the "summary" field in {language}. This overrides any other instruction about language."""

TRANSLATION_RULE_SAME = """[Translation Rule]
The user's target language is "{target}". If the email is already in {target_name}, set "translation" to null.
If the email is in a DIFFERENT language, provide a faithful translation of the core content into {target_name} in the "translation" field."""

TRANSLATION_RULE_AUTO = """[Translation Rule]
The user's language mode is "auto" — set "translation" to null. The summary should be in the same language as the email."""

USER_PROMPT_TEMPLATE = """Analyze the email:

From: {sender}
Subject: {subject}
Body:
{body}"""

# JSON Schema for structured output enforcement
ANALYSIS_SCHEMA = {
    "type": "object",
    "properties": {
        "summary": {"type": "string", "description": "Concise summary, 50-100 words"},
        "category": {
            "type": "string",
            "enum": [
                "verification_code",
                "notification",
                "billing",
                "promotion",
                "personal",
            ],
        },
        "priority": {"type": "integer", "minimum": 1, "maximum": 5},
        "extracted_code": {
            "type": ["string", "null"],
            "description": "Extracted verification code or token, null if none",
        },
        "source_language": {
            "type": "string",
            "description": "ISO 639-1 code of the email's primary language",
        },
        "translation": {
            "type": ["string", "null"],
            "description": "Translation of key content when languages differ, null otherwise",
        },
    },
    "required": ["summary", "category", "priority", "extracted_code", "source_language", "translation"],
    "additionalProperties": False,
}


# ──────────────────────────────────────────────
#  Prompt construction  (Feat 3: Runtime Injection)
# ──────────────────────────────────────────────

def build_system_prompt(config: AIConfig, rules_block: str | None = None) -> str:
    """
    Assemble the final system prompt following priority order:

        1.  JSON config constraints  (highest — language, mode)
        2.  Markdown persona rules   (medium)
        3.  Base system prompt       (lowest — default behaviour)

    Conflict rule: JSON config > Markdown rules > Base prompt.
    """
    sections: list[str] = []

    # --- Layer 1: System-level constraint from JSON config (highest priority) ---
    lang = config.language or "auto"
    lang_name = LANGUAGE_NAMES.get(lang, lang)
    if lang != "auto":
        sections.append(LANGUAGE_CONSTRAINT_TEMPLATE.format(language=lang_name))
        sections.append(
            TRANSLATION_RULE_SAME.format(target=lang, target_name=lang_name)
        )
    else:
        sections.append(TRANSLATION_RULE_AUTO)

    # --- Layer 2: Persona rules from Markdown (medium priority) ---
    if rules_block:
        sections.append(rules_block)

    # --- Layer 3: Base system prompt (lowest priority) ---
    sections.append(BASE_SYSTEM_PROMPT)

    return "\n\n".join(sections)


# ──────────────────────────────────────────────
#  Core helpers
# ──────────────────────────────────────────────

@contextmanager
def _bypass_socket_proxy() -> Iterator[None]:
    """Temporarily restore the original (un-patched) socket.

    ``apply_global_proxy`` monkey-patches ``socket.socket`` with PySocks so that
    IMAP connections are transparently proxied.  However, httpx (used by litellm)
    already honours ``HTTP(S)_PROXY`` env-vars and sets up its own proxy tunnel.
    If the socket is *also* patched, httpx ends up double-proxying the connection
    (socket-level proxy → HTTP-level proxy), which causes an SSL EOF error.

    This context manager swaps the original socket back for the duration of the
    ``with`` block so that httpx/litellm use only env-var-based proxying.
    """
    from utils.helpers import _ORIGINAL_SOCKET

    patched = socket.socket
    socket.socket = _ORIGINAL_SOCKET
    try:
        yield
    finally:
        socket.socket = patched


def _patch_litellm_cost_map(exc: FileNotFoundError) -> None:
    """Create missing litellm JSON data files so the module can be re-imported.

    In a PyInstaller --onefile bundle the temporary extraction directory may
    lack non-Python resource files that litellm reads at import time.  This
    helper writes an empty ``{}`` placeholder so the second import succeeds.
    """
    missing = getattr(exc, "filename", "") or ""
    if "model_prices_and_context_window" in missing:
        path = Path(missing)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("{}", encoding="utf-8")
        logger.debug("Created placeholder litellm cost map: %s", path)
    else:
        logger.debug("Unexpected litellm FileNotFoundError: %s", exc)


def _is_pyinstaller_bundle() -> bool:
    """Return True when running inside a PyInstaller frozen bundle."""
    return getattr(sys, "frozen", False) and hasattr(sys, "_MEIPASS")


def _default_result() -> AIAnalysisResult:
    """Return a safe default when AI analysis fails."""
    return AIAnalysisResult()


def _build_litellm_params(config: AIConfig) -> dict:
    """Build kwargs dict for litellm.completion based on provider config."""
    params: dict = {
        "model": config.model,
    }

    if config.api_key:
        params["api_key"] = config.api_key.get_secret_value()

    if config.base_url:
        params["api_base"] = config.base_url

    # Provider-specific model prefix for litellm routing
    model = config.model
    provider = config.provider.lower()
    prefix_map = {
        "deepseek": "deepseek/",
        "ollama": "ollama/",
        "ollama_chat": "ollama_chat/",
    }

    prefix = prefix_map.get(provider)
    if prefix and not model.startswith(prefix):
        params["model"] = f"{prefix}{model}"

    return params


def _parse_response(raw_text: str) -> AIAnalysisResult:
    """Parse LLM response text into AIAnalysisResult, handling markdown fences."""
    text = raw_text.strip()

    # Strip markdown code fences if present
    if text.startswith("```"):
        text = re.sub(r"^```(?:json)?\s*", "", text)
        text = re.sub(r"\s*```$", "", text)

    data = json.loads(text)
    return AIAnalysisResult.model_validate(data)


# ──────────────────────────────────────────────
#  Public API
# ──────────────────────────────────────────────

def analyze_email(
    subject: str,
    sender: str,
    body: str,
    config: AIConfig,
    rules_block: str | None = None,
) -> AIAnalysisResult:
    """
    Analyze email content using LLM and return structured result.

    Falls back to default result on any error to keep core forwarding alive.

    Args:
        subject: email subject
        sender: sender address
        body: cleaned plain-text body
        config: AI configuration
        rules_block: optional persona rules prompt block from RulesManager

    Returns:
        AIAnalysisResult (may be default on failure)
    """
    if not config.enabled:
        logger.debug("AI analysis disabled, returning default")
        return _default_result()

    try:
        import litellm  # lazy import to avoid hard dependency

        from utils.logger import adopt_dependency_loggers, get_active_log_level

        litellm.drop_params = True  # ignore unsupported params per provider
        litellm.aiohttp_trust_env = True  # honor HTTP_PROXY / HTTPS_PROXY as per docs
        adopt_dependency_loggers(("LiteLLM",), level=get_active_log_level(), force_handlers=False)
    except ImportError:
        logger.error("litellm not installed — run: pip install litellm")
        return _default_result()
    except FileNotFoundError as exc:
        # PyInstaller bundle may lack litellm data files; patch and retry.
        _patch_litellm_cost_map(exc)
        try:
            import litellm  # type: ignore[reimported]  # noqa: F811

            litellm.drop_params = True
        except Exception:
            logger.error("litellm data files missing and could not be recreated")
            return _default_result()

    # Truncate body to ~2000 chars for cost and latency
    truncated_body = body[:2000] if len(body) > 2000 else body

    # Build dynamic system prompt (Feat 3: config → rules → base)
    system_prompt = build_system_prompt(config, rules_block)

    user_msg = USER_PROMPT_TEMPLATE.format(
        sender=sender,
        subject=subject,
        body=truncated_body,
    )

    params = _build_litellm_params(config)

    try:
        with _bypass_socket_proxy():
            response = litellm.completion(
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_msg},
                ],
                response_format={"type": "json_object"},
                temperature=0.3,
                max_tokens=800,
                timeout=30,
                **params,
            )

        raw = response.choices[0].message.content  # type: ignore[union-attr]
        if not raw:
            logger.warning("LLM returned empty content")
            return _default_result()

        result = _parse_response(raw)
        logger.info(
            "AI analysis OK: category=%s priority=%d source_lang=%s",
            result.category, result.priority, result.source_language,
        )
        return result

    except json.JSONDecodeError as e:
        logger.warning("Failed to parse LLM JSON output: %s", e)
        return _default_result()
    except Exception:
        logger.exception("AI analysis failed, returning default")
        return _default_result()


def should_skip_ai(body: str) -> bool:
    """
    Hybrid mode heuristic: return True if the email is short or matches
    common patterns that don't need AI (verification codes, login alerts).

    Used in Mode B to decide whether to call AI automatically.
    """
    if len(body) < 100:
        return True

    # Quick regex check for common short-action patterns
    skip_patterns = re.compile(
        r"(验证码|verification\s*code|登录|login|sign.?in|code\s*[:：])",
        re.IGNORECASE,
    )
    if skip_patterns.search(body[:300]):
        return True

    return False

def detect_language_simple(text: str) -> str | None:
    """
    Simple heuristic language detection without external dependencies.
    Returns ISO 639-1 code (e.g., 'zh', 'en', 'ja') or None if uncertain.
    
    Used in Hybrid mode to decide whether to show Translate button.
    Not accurate, but sufficient for button visibility logic.
    
    Args:
        text: Email body text to analyze (will sample first 1000 chars)
    
    Returns:
        Language code or None
    """
    if not text:
        return None
    
    sample = text[:1000]
    
    # Count character types
    cjk_count = sum(1 for c in sample if '\u4e00' <= c <= '\u9fff' or '\u3040' <= c <= '\u309f' or '\u30a0' <= c <= '\u30ff' or '\uac00' <= c <= '\ud7af')
    arabic_count = sum(1 for c in sample if '\u0600' <= c <= '\u06ff')
    cyrillic_count = sum(1 for c in sample if '\u0400' <= c <= '\u04ff')
    latin_count = sum(1 for c in sample if ord(c) < 128 and c.isalpha())
    
    total_chars = len([c for c in sample if c.isalpha()])
    
    if total_chars == 0:
        return None
    
    # Determine language by highest character type ratio
    cjk_ratio = cjk_count / total_chars
    arabic_ratio = arabic_count / total_chars
    cyrillic_ratio = cyrillic_count / total_chars
    latin_ratio = latin_count / total_chars
    
    # Detect CJK (Chinese, Japanese, Korean)
    if cjk_ratio > 0.3:
        # Try to distinguish between Chinese, Japanese, Korean
        hiragana_katakana = sum(1 for c in sample if '\u3040' <= c <= '\u309f' or '\u30a0' <= c <= '\u30ff')
        hangul = sum(1 for c in sample if '\uac00' <= c <= '\ud7af')
        hanzi = sum(1 for c in sample if '\u4e00' <= c <= '\u9fff')
        
        if hiragana_katakana > hanzi:
            return 'ja'  # Japanese (has hiragana/katakana)
        elif hangul > hanzi:
            return 'ko'  # Korean (has Hangul)
        else:
            return 'zh'  # Chinese (has Hanzi)
    
    # Detect Arabic
    if arabic_ratio > 0.3:
        return 'ar'
    
    # Detect Russian/Cyrillic
    if cyrillic_ratio > 0.3:
        return 'ru'
    
    # Default to English for Latin-script languages
    if latin_ratio > 0.7:
        return 'en'
    
    return None
