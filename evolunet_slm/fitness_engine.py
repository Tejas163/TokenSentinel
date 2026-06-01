"""HuggingFace model loading and evolutionary fitness evaluation."""

import logging
import re
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable, List, Optional

from config import CPUConfig, InferenceConfig, ModelConfig

logger = logging.getLogger(__name__)


class HFModelLoader:
    """CPU-optimized HuggingFace model loader for prompt evaluation.

    Uses transformers + torch with CPU settings from config.
    """

    def __init__(
        self,
        model_cfg: ModelConfig,
        cpu_cfg: CPUConfig,
        infer_cfg: InferenceConfig,
    ):
        self.model_cfg = model_cfg
        self.cpu_cfg = cpu_cfg
        self.infer_cfg = infer_cfg
        self.model = None
        self.tokenizer = None

    def load(self):
        from transformers import AutoModelForCausalLM, AutoTokenizer
        model_path = str(self.model_cfg.model_dir)
        if not self.model_cfg.model_dir.exists():
            raise FileNotFoundError(
                f"Model directory not found at {model_path}. "
                f"Run `python download_model.py` first."
            )
        logger.info("Loading model from %s ...", model_path)
        self.tokenizer = AutoTokenizer.from_pretrained(
            model_path, local_files_only=True
        )
        self.model = AutoModelForCausalLM.from_pretrained(
            model_path,
            local_files_only=True,
            torch_dtype="auto",
            low_cpu_mem_usage=True,
        )
        self.model.eval()
        logger.info("Model loaded successfully.")
        return self.model

    def unload(self):
        self.model = None
        self.tokenizer = None

    def generate(self, prompt: str, **overrides) -> dict:
        import torch
        if self.model is None:
            self.load()
        max_tokens = overrides.get("max_tokens", self.infer_cfg.max_tokens)
        temperature = overrides.get("temperature", self.infer_cfg.temperature)
        top_p = overrides.get("top_p", self.infer_cfg.top_p)
        top_k = overrides.get("top_k", self.infer_cfg.top_k)
        repeat_penalty = overrides.get("repeat_penalty", self.infer_cfg.repeat_penalty)

        inputs = self.tokenizer(prompt, return_tensors="pt")
        input_len = inputs["input_ids"].shape[1]

        with torch.no_grad():
            outputs = self.model.generate(
                **inputs,
                max_new_tokens=max_tokens,
                temperature=temperature,
                top_p=top_p,
                top_k=top_k,
                repetition_penalty=repeat_penalty,
                do_sample=temperature > 0,
                pad_token_id=self.tokenizer.eos_token_id,
            )

        generated = outputs[0][input_len:]
        text = self.tokenizer.decode(generated, skip_special_tokens=True)
        output_len = len(generated)

        return {
            "choices": [{"text": text}],
            "usage": {
                "prompt_tokens": input_len,
                "completion_tokens": output_len,
            },
        }


@dataclass
class FitnessResult:
    """Result of evaluating a single prompt."""

    prompt: str
    response: str
    fitness: float
    prompt_tokens: int
    completion_tokens: int
    latency_ms: float
    response_length: int


def default_fitness_scorer(response: str, prompt_tokens: int, completion_tokens: int) -> float:
    """Score a generated response.

    Higher is better. Rewards:
    - Longer substantive responses (up to a point)
    - Structure (bullet points, numbered lists indicate organization)
    - Penalizes verbosity via token cost
    """
    response_len = len(response.split())
    structure_bonus = 0
    if "- " in response:
        structure_bonus += 0.1
    if re.search(r"\d+\.\s", response):
        structure_bonus += 0.1
    if "**" in response:
        structure_bonus += 0.05
    length_score = response_len / (response_len + 50)
    token_penalty = 1.0 - (completion_tokens / 500.0) * 0.2
    token_penalty = max(token_penalty, 0.5)
    return (length_score + structure_bonus) * token_penalty


class EvaluationLoop:
    """Batch evaluation of prompt variants using the HF model."""

    def __init__(
        self,
        loader: HFModelLoader,
        scorer: Optional[Callable[[str, int, int], float]] = None,
        target_prompt: str = "",
    ):
        self.loader = loader
        self.scorer = scorer or default_fitness_scorer
        self.target_prompt = target_prompt

    def evaluate(self, prompts: List[str]) -> List[FitnessResult]:
        results: List[FitnessResult] = []
        for prompt in prompts:
            start = time.time()
            raw = self.loader.generate(prompt)
            elapsed = (time.time() - start) * 1000
            usage = raw.get("usage", {})
            prompt_tokens = usage.get("prompt_tokens", 0)
            completion_tokens = usage.get("completion_tokens", 0)
            response = raw["choices"][0]["text"].strip()
            fitness = self.scorer(response, prompt_tokens, completion_tokens)
            results.append(FitnessResult(
                prompt=prompt,
                response=response,
                fitness=fitness,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                latency_ms=elapsed,
                response_length=len(response.split()),
            ))
        return results
