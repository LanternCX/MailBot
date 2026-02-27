"""Event contracts shared by core and service layers."""

from __future__ import annotations

from dataclasses import dataclass

from core.models import AIAnalysisResult, EmailSnapshot


@dataclass(frozen=True)
class EmailReceived:
    email: EmailSnapshot


@dataclass(frozen=True)
class ForwardRequested:
    email: EmailSnapshot


@dataclass(frozen=True)
class SummaryRequested:
    email: EmailSnapshot


@dataclass(frozen=True)
class SummaryReady:
    email: EmailSnapshot
    result: AIAnalysisResult
