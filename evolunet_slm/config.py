"""CPU-optimized configuration for EvoluNet-SLM."""

import multiprocessing
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class CPUConfig:
    """Hardware-aware settings tuned for CPU-only inference."""

    n_threads: int = field(default_factory=lambda: max(1, multiprocessing.cpu_count() - 1))
    n_batch: int = 8
    n_ubatch: int = 8
    use_mmap: bool = True
    use_mlock: bool = False
    seed: int = 42
    verbose: bool = False


@dataclass
class InferenceConfig:
    """Generation parameters for the SLM."""

    n_ctx: int = 2048
    max_tokens: int = 256
    temperature: float = 0.7
    top_p: float = 0.9
    top_k: int = 40
    repeat_penalty: float = 1.1
    stop: list[str] = field(default_factory=lambda: ["<|im_end|>", "\n\n---"])


@dataclass
class ModelConfig:
    """Paths and model identity."""

    repo_id: str = "Qwen/Qwen2-0.5B-Instruct-GGUF"
    filename: str = "qwen2-0_5b-instruct-q4_k_m.gguf"
    models_dir: Path = Path("models")
    local_model_path: Path | None = None

    def __post_init__(self):
        self.local_model_path = self.models_dir / self.filename

