"""PyInstaller hook for tiktoken — collect the native extension and plugin data."""

from __future__ import annotations

from PyInstaller.utils.hooks import collect_data_files, collect_submodules

# tiktoken uses a native Rust extension (_tiktoken) and discovers encodings
# via the tiktoken_ext namespace package.  Both must be collected explicitly.
datas = collect_data_files("tiktoken") + collect_data_files("tiktoken_ext")
hiddenimports = collect_submodules("tiktoken") + collect_submodules("tiktoken_ext")
