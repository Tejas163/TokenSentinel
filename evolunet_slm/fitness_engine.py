"""GGUF model loading with CPU-optimized settings — basic generation test."""

from pathlib import Path

from llama_cpp import Llama

from config import CPUConfig, InferenceConfig, ModelConfig


class GGUFModelLoader:
    def __init__(
        self,
        model_cfg: ModelConfig,
        cpu_cfg: CPUConfig,
        infer_cfg: InferenceConfig,
    ):
        self.model_cfg = model_cfg
        self.cpu_cfg = cpu_cfg
        self.infer_cfg = infer_cfg
        self.llm: Llama | None = None

    def load(self):
        model_path = self._resolve_model_path()
        self.llm = Llama(
            model_path=str(model_path),
            n_ctx=self.infer_cfg.n_ctx,
            n_threads=self.cpu_cfg.n_threads,
            n_batch=self.cpu_cfg.n_batch,
            n_ubatch=self.cpu_cfg.n_ubatch,
            use_mmap=self.cpu_cfg.use_mmap,
            use_mlock=self.cpu_cfg.use_mlock,
            seed=self.cpu_cfg.seed,
            verbose=self.cpu_cfg.verbose,
        )
        return self.llm

    def unload(self):
        self.llm = None

    def _resolve_model_path(self) -> Path:
        path = self.model_cfg.local_model_path
        if path and path.exists():
            return path
        raise FileNotFoundError(
            f"GGUF model not found at {path}. "
            f"Run `python download_model.py` first."
        )

    def generate(self, prompt: str, **overrides) -> dict:
        if self.llm is None:
            self.load()
        kwargs = {
            "prompt": prompt,
            "max_tokens": self.infer_cfg.max_tokens,
            "temperature": self.infer_cfg.temperature,
            "top_p": self.infer_cfg.top_p,
            "top_k": self.infer_cfg.top_k,
            "repeat_penalty": self.infer_cfg.repeat_penalty,
            "stop": self.infer_cfg.stop,
            "echo": False,
        }
        kwargs.update(overrides)
        return self.llm(**kwargs)


if __name__ == "__main__":
    model_cfg = ModelConfig()
    cpu_cfg = CPUConfig()
    infer_cfg = InferenceConfig()

    loader = GGUFModelLoader(model_cfg, cpu_cfg, infer_cfg)

    print(f"Loading model from {model_cfg.local_model_path} ...")
    print(f"CPU threads: {cpu_cfg.n_threads}, use_mmap: {cpu_cfg.use_mmap}")

    loader.load()
    print("Model loaded successfully.\n")

    result = loader.generate("The meaning of life is")
    print(f"Generated text:\n{result['choices'][0]['text'].strip()}")
