"""AI package exports analysis and gateway APIs."""

from core.ai.analysis import (
    ANALYSIS_SCHEMA,
    BASE_SYSTEM_PROMPT,
    LANGUAGE_NAMES,
    USER_PROMPT_TEMPLATE,
    _bypass_socket_proxy,
    analyze_email,
    build_system_prompt,
    detect_language_simple,
    should_skip_ai,
)

__all__ = [
    "ANALYSIS_SCHEMA",
    "BASE_SYSTEM_PROMPT",
    "LANGUAGE_NAMES",
    "USER_PROMPT_TEMPLATE",
    "_bypass_socket_proxy",
    "analyze_email",
    "build_system_prompt",
    "detect_language_simple",
    "should_skip_ai",
]
