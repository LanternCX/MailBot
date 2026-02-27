"""Structural typing protocols for chat adapters."""

from __future__ import annotations

from collections.abc import Callable
from typing import Any, Protocol

CommandHandler = Callable[[str, str], None]
CallbackHandler = Callable[[str, str, str, int], None]


class ChatBotProtocol(Protocol):
    command_handlers: dict[str, CommandHandler]
    callback_handlers: dict[str, CallbackHandler]

    def start_polling(self) -> None: ...

    def stop(self) -> None: ...

    def send_message(self, chat_id: str, text: str, **kwargs: Any) -> bool: ...
