"""Package MailBot with PyInstaller and prepare release archives."""

from __future__ import annotations

import argparse
import json
import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path


def _litellm_data_args() -> list[str]:
    """Return --add-data args for litellm cost map files if present."""
    try:
        import litellm
    except ImportError:
        return []

    data_dir = Path(litellm.__file__).parent
    files = [
        data_dir / "model_prices_and_context_window_backup.json",
        data_dir / "model_prices_and_context_window.json",
    ]

    sep = ";" if os.name == "nt" else ":"
    args: list[str] = []
    for src in files:
        if src.exists():
            args.extend(["--add-data", f"{src}{sep}litellm/{src.name}"])
    return args


def _litellm_pyinstaller_args() -> list[str]:
    """Return pyinstaller args to ensure litellm modules and data are bundled."""
    try:
        import litellm  # noqa: F401
    except ImportError:
        return []

    args: list[str] = [
        "--hidden-import",
        "litellm",
        "--collect-data",
        "litellm",
        # tiktoken is used by litellm for token counting; its encoding
        # registry relies on the tiktoken_ext namespace package which
        # PyInstaller cannot discover automatically.
        "--hidden-import",
        "tiktoken",
        "--hidden-import",
        "tiktoken_ext",
        "--hidden-import",
        "tiktoken_ext.openai_public",
        "--collect-data",
        "tiktoken_ext",
        # socksio is needed by httpx for SOCKS proxy support;
        # it is a lazy/optional import that PyInstaller cannot detect.
        "--hidden-import",
        "socksio",
    ]
    args += _litellm_data_args()
    return args


def main() -> int:
    parser = argparse.ArgumentParser(description="Build a release bundle for MailBot")
    parser.add_argument("--entry", default="main.py", help="Entry-point script")
    parser.add_argument("--name", default="MailBot", help="Executable base name")
    parser.add_argument("--variant", default="linux-x64", help="Platform tag, e.g. macos-arm64")
    parser.add_argument("--tag", default="", help="Release tag, e.g. v1.9.1 (optional)")
    parser.add_argument(
        "--clean",
        action="store_true",
        help="Remove build artifacts before packaging",
    )
    args = parser.parse_args()

    system = platform.system()
    machine = platform.machine() or "unknown"

    variant = args.variant or f"{system}-{machine}"
    tag = args.tag.strip()

    dist_root = Path("dist")
    dist_target = dist_root / variant
    build_root = Path("build") / variant

    if args.clean:
        shutil.rmtree(dist_root, ignore_errors=True)
        shutil.rmtree(build_root, ignore_errors=True)

    hooks_dir = Path(__file__).resolve().parent.parent / "hooks"

    pyinstaller_cmd = [
        sys.executable,
        "-m",
        "PyInstaller",
        "--name",
        args.name,
        "--onefile",
        "--distpath",
        str(dist_target),
        "--workpath",
        str(build_root),
        "--specpath",
        str(build_root),
    ]

    if hooks_dir.is_dir():
        pyinstaller_cmd += ["--additional-hooks-dir", str(hooks_dir)]

    pyinstaller_cmd.append(args.entry)

    # Include litellm cost map JSON files so packaged binary does not crash when loading pricing metadata.
    pyinstaller_cmd += _litellm_pyinstaller_args()

    print("Running PyInstaller:", "\"" + " ".join(pyinstaller_cmd) + "\"")
    result = subprocess.run(pyinstaller_cmd, check=False)
    if result.returncode != 0:
        print("PyInstaller failed.")
        return result.returncode

    if not dist_target.exists():
        print(f"Expected dist directory does not exist: {dist_target}")
        return 1

    # Ensure a default config.json is shipped so the binary can start with sane defaults
    _ensure_default_config(dist_target)

    archive_name = f"{tag}-{variant}" if tag else f"{args.name}-{variant}"
    archive_base = dist_root / archive_name
    shutil.make_archive(str(archive_base), "zip", root_dir=dist_target)

    print("Release bundle created:", f"{archive_base}.zip")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())


def _ensure_default_config(dist_target: Path) -> None:
    """Write a minimal config.json into the dist folder if absent."""
    dist_target.mkdir(parents=True, exist_ok=True)
    cfg_path = dist_target / "config.json"
    if cfg_path.exists():
        return
    default_cfg = {
        "poll_interval": 60,
        "max_retries": 3,
        "log_level": "INFO",
        "accounts": [],
        "notifiers": [],
    }
    cfg_path.write_text(json.dumps(default_cfg, indent=2), encoding="utf-8")
