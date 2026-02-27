"""Abstract chat adapter base class."""

from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Callable
from typing import Any

CommandHandler = Callable[[str, str], None]
CallbackHandler = Callable[[str, str, str, int], None]


class ChatBotBase(ABC):
    @abstractmethod
    def start_polling(self) -> None: ...

    @abstractmethod
    def stop(self) -> None: ...

    @abstractmethod
    def send_message(self, chat_id: str, text: str, **kwargs: Any) -> bool: ...

    @abstractmethod
    def edit_message(
        self, chat_id: str, message_id: int, text: str, **kwargs: Any
    ) -> bool: ...

    @abstractmethod
    def answer_callback(self, callback_query_id: str, text: str = "") -> bool: ...

    @abstractmethod
    def on_command(self, name: str, handler: CommandHandler) -> None: ...

    @abstractmethod
    def on_callback(self, prefix: str, handler: CallbackHandler) -> None: ...
