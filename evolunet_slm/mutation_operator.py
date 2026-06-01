"""Evolutionary prompt mutation operators."""

import random
from typing import List, Tuple

INSTRUCTIONAL_PHRASES = [
    "Think step by step.",
    "Be concise.",
    "Explain like I'm 5.",
    "Provide examples.",
    "Justify your reasoning.",
    "Consider edge cases.",
    "Be thorough.",
    "Summarize at the end.",
    "Use bullet points.",
    "Write in plain English.",
    "Assume I'm a beginner.",
    "Cite your sources.",
    "Consider the opposite view.",
    "List pros and cons.",
    "Start with the bottom line.",
]

SYNONYM_MAP = {
    "explain": ["describe", "clarify", "elaborate", "break down"],
    "list": ["enumerate", "catalog", "itemize", "mention"],
    "write": ["compose", "draft", "produce", "generate"],
    "give": ["provide", "offer", "present", "supply"],
    "use": ["apply", "employ", "utilize", "leverage"],
    "good": ["effective", "sound", "solid", "strong"],
    "bad": ["poor", "weak", "flawed", "suboptimal"],
    "big": ["large", "substantial", "significant", "major"],
    "small": ["minor", "tiny", "modest", "limited"],
    "fast": ["quick", "rapid", "swift", "efficient"],
    "slow": ["gradual", "measured", "deliberate", "leisurely"],
    "simple": ["straightforward", "basic", "easy", "plain"],
    "complex": ["complicated", "intricate", "sophisticated", "nuanced"],
    "important": ["critical", "essential", "crucial", "key"],
    "find": ["identify", "locate", "determine", "discover"],
    "show": ["demonstrate", "illustrate", "display", "reveal"],
    "help": ["assist", "aid", "support", "guide"],
    "make": ["create", "build", "construct", "form"],
    "tell": ["inform", "advise", "notify", "describe to"],
}


def crossover_single_point(parent_a: str, parent_b: str) -> Tuple[str, str]:
    tokens_a = parent_a.split()
    tokens_b = parent_b.split()
    if len(tokens_a) < 2 or len(tokens_b) < 2:
        return parent_a, parent_b
    point = random.randint(1, min(len(tokens_a), len(tokens_b)) - 1)
    child_a = " ".join(tokens_a[:point] + tokens_b[point:])
    child_b = " ".join(tokens_b[:point] + tokens_a[point:])
    return child_a, child_b


def crossover_uniform(parent_a: str, parent_b: str, prob: float = 0.5) -> Tuple[str, str]:
    tokens_a = parent_a.split()
    tokens_b = parent_b.split()
    max_len = max(len(tokens_a), len(tokens_b))
    while len(tokens_a) < max_len:
        tokens_a.append("")
    while len(tokens_b) < max_len:
        tokens_b.append("")
    child_a, child_b = [], []
    for ta, tb in zip(tokens_a, tokens_b):
        if random.random() < prob:
            child_a.append(tb)
            child_b.append(ta)
        else:
            child_a.append(ta)
            child_b.append(tb)
    return " ".join(child_a).strip(), " ".join(child_b).strip()


def mutate_substitute(phrase: str, rate: float = 0.1) -> str:
    tokens = phrase.split()
    result = []
    for token in tokens:
        clean = token.lower().strip(".,!?;:")
        punct = token[len(clean):] if len(token) > len(clean) else ""
        if clean in SYNONYM_MAP and random.random() < rate:
            synonym = random.choice(SYNONYM_MAP[clean])
            if token[0].isupper():
                synonym = synonym.capitalize()
            result.append(synonym + punct)
        else:
            result.append(token)
    return " ".join(result)


def mutate_insert(phrase: str, rate: float = 0.05) -> str:
    tokens = phrase.split()
    if not tokens:
        return phrase
    if random.random() < rate:
        insert_pos = random.randint(1, len(tokens))
        instruction = random.choice(INSTRUCTIONAL_PHRASES)
        tokens.insert(insert_pos, instruction)
    return " ".join(tokens)


def mutate_delete(phrase: str, rate: float = 0.05) -> str:
    tokens = phrase.split()
    if len(tokens) <= 3:
        return phrase
    survivors = [t for t in tokens if random.random() >= rate]
    return " ".join(survivors) if survivors else phrase


def mutate_shuffle(phrase: str, rate: float = 0.1) -> str:
    tokens = phrase.split()
    if len(tokens) < 2:
        return phrase
    for i in range(len(tokens) - 1):
        if random.random() < rate:
            tokens[i], tokens[i + 1] = tokens[i + 1], tokens[i]
    return " ".join(tokens)
