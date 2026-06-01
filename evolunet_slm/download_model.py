"""Download a lightweight 4-bit CPU GGUF model from Hugging Face."""

from pathlib import Path

from huggingface_hub import hf_hub_download


def download_qwen_gguf(repo_id: str = "Qwen/Qwen2-0.5B-Instruct-GGUF",
                       filename: str = "qwen2-0_5b-instruct-q4_k_m.gguf",
                       local_dir: str = "models") -> Path:
    local_dir = Path(local_dir)
    local_dir.mkdir(parents=True, exist_ok=True)

    model_path = hf_hub_download(
        repo_id=repo_id,
        filename=filename,
        local_dir=str(local_dir),
        resume_download=True,
    )
    return Path(model_path)


if __name__ == "__main__":
    path = download_qwen_gguf()
    print(f"Model downloaded to: {path}")
    print(f"Size: {path.stat().st_size / 1024**3:.2f} GiB")
