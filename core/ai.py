"""
core/ai.py
~~~~~~~~~~
AI-powered email analysis using LiteLLM for multi-provider LLM routing.

Responsibilities:
- Analyze email content and produce structured output
- Support multiple LLM providers via litellm
- Graceful fallback on API errors
"""

from __future__ import annotations

import json
import logging
import re

from core.models import AIAnalysisResult, AIConfig

logger = logging.getLogger("mailbot.ai")

# System prompt for email analysis
SYSTEM_PROMPT = """You are a professional mail triage assistant. Analyze the email content and return a JSON object that strictly matches the provided schema.

Requirements:
1. summary: English summary in 50-100 words, focusing on the main intent
2. category: choose one of [verification_code, notification, billing, promotion, personal]
3. priority: integer 1-5 (1=urgent, 5=low)
    - verification_code / security → 1
    - personal / important notification → 2
    - billing / subscription → 3
    - general notification → 4
    - promotion / marketing → 5
4. extracted_code: if the email contains a verification/pickup/auth code, extract it; otherwise use null

Return only a single JSON object without extra text or markdown."""

USER_PROMPT_TEMPLATE = """Analyze the email:

From: {sender}
Subject: {subject}
Body:
{body}"""

# JSON Schema for structured output enforcement
ANALYSIS_SCHEMA = {
    "type": "object",
    "properties": {
        "summary": {"type": "string", "description": "Chinese summary, 50-100 chars"},
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
    },
    "required": ["summary", "category", "priority", "extracted_code"],
    "additionalProperties": False,
}


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
    if provider == "deepseek" and not model.startswith("deepseek/"):
        params["model"] = f"deepseek/{model}"
    elif provider == "ollama" and not model.startswith("ollama/"):
        params["model"] = f"ollama/{model}"
    elif provider == "ollama_chat" and not model.startswith("ollama_chat/"):
        params["model"] = f"ollama_chat/{model}"

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


def analyze_email(
    subject: str,
    sender: str,
    body: str,
    config: AIConfig,
) -> AIAnalysisResult:
    """
    Analyze email content using LLM and return structured result.

    Falls back to default result on any error to keep core forwarding alive.

    Args:
        subject: email subject
        sender: sender address
        body: cleaned plain-text body
        config: AI configuration

    Returns:
        AIAnalysisResult (may be default on failure)
    """
    if not config.enabled:
        logger.debug("AI analysis disabled, returning default")
        return _default_result()

    try:
        import litellm  # lazy import to avoid hard dependency

        litellm.drop_params = True  # ignore unsupported params per provider
    except ImportError:
        logger.error("litellm not installed — run: pip install litellm")
        return _default_result()

    # Truncate body to ~2000 chars for cost and latency
    truncated_body = body[:2000] if len(body) > 2000 else body

    user_msg = USER_PROMPT_TEMPLATE.format(
        sender=sender,
        subject=subject,
        body=truncated_body,
    )

    params = _build_litellm_params(config)

    try:
        response = litellm.completion(
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": user_msg},
            ],
            response_format={"type": "json_object"},
            temperature=0.3,
            max_tokens=500,
            timeout=30,
            **params,
        )

        raw = response.choices[0].message.content  # type: ignore[union-attr]
        if not raw:
            logger.warning("LLM returned empty content")
            return _default_result()

        result = _parse_response(raw)
        logger.info("AI analysis OK: category=%s priority=%d", result.category, result.priority)
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
