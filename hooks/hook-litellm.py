"""PyInstaller hook for litellm — collect data files (cost maps) and submodules."""

from __future__ import annotations

from PyInstaller.utils.hooks import collect_data_files, collect_submodules

datas = collect_data_files("litellm")
hiddenimports = collect_submodules("litellm") + [
    "tiktoken",
    "tiktoken_ext",
    "tiktoken_ext.openai_public",
    "socksio",
]
