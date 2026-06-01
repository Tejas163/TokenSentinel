"""Download Qwen2-0.5B-Instruct (safetensors) for transformers-based inference."""

import logging
from pathlib import Path

from huggingface_hub import snapshot_download
from config import ModelConfig

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def main():
    cfg = ModelConfig()
    target = cfg.models_dir / "qwen2-0_5b-instruct"
    target.mkdir(parents=True, exist_ok=True)

    logger.info("Downloading Qwen/Qwen2-0.5B-Instruct (safetensors) ...")
    snapshot_download(
        repo_id="Qwen/Qwen2-0.5B-Instruct",
        local_dir=str(target),
        local_dir_use_symlinks=False,
        ignore_patterns=["*.gguf"],
    )

    size_mb = sum(
        f.stat().st_size for f in target.rglob("*") if f.is_file()
    ) / (1024 * 1024)
    logger.info("Downloaded to %s (%.1f MiB)", target, size_mb)


if __name__ == "__main__":
    main()
