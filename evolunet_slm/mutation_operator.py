import random


def crossover_single_point(parent_a: str, parent_b: str) -> tuple[str, str]:
    """Swap segments between two prompts at a random token boundary."""
    raise NotImplementedError


def crossover_uniform(parent_a: str, parent_b: str, prob: float = 0.5) -> tuple[str, str]:
    """Blend tokens from both parents with per-token probability."""
    raise NotImplementedError


def mutate_substitute(phrase: str, rate: float = 0.1) -> str:
    """Replace random tokens with synonyms or placeholder markers."""
    raise NotImplementedError


def mutate_insert(phrase: str, rate: float = 0.05) -> str:
    """Insert extra instructional tokens at random positions."""
    raise NotImplementedError


def mutate_delete(phrase: str, rate: float = 0.05) -> str:
    """Drop random tokens from the prompt."""
    raise NotImplementedError


def mutate_shuffle(phrase: str, rate: float = 0.1) -> str:
    """Rearrange adjacent token pairs."""
    raise NotImplementedError
