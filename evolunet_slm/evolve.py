"""EvoluNet-SLM orchestrator: evolutionary prompt optimization with cost tracking."""

import logging
import random
import sys
import time
from dataclasses import dataclass, field
from typing import List, Optional, Tuple

from fitness_engine import EvaluationLoop, FitnessResult, HFModelLoader
from mutation_operator import (
    crossover_single_point,
    mutate_substitute,
    mutate_insert,
    mutate_delete,
    mutate_shuffle,
)
from config import CPUConfig, InferenceConfig, ModelConfig

logger = logging.getLogger(__name__)


# Approximate per-token cost in USD (gpt-3.5-turbo pricing as proxy for prompt eval)
COST_PER_INPUT_TOKEN = 0.0000015
COST_PER_OUTPUT_TOKEN = 0.0000020

SEED_PROMPTS = [
    "Explain the concept in simple terms.",
    "Break down the topic step by step with examples.",
    "Provide a thorough analysis covering advantages and disadvantages.",
    "Summarize the key points concisely.",
    "Write a beginner-friendly explanation with analogies.",
    "Give a detailed technical explanation suitable for an expert audience.",
    "Compare and contrast the different approaches with pros and cons.",
    "Walk through the solution methodically, showing your reasoning.",
]


@dataclass
class EvolveConfig:
    population_size: int = 20
    generations: int = 10
    mutation_rate: float = 0.15
    crossover_rate: float = 0.7
    elite_ratio: float = 0.1
    tournament_size: int = 3
    model_path: str = "models/qwen2-0_5b-instruct-q4_k_m.gguf"
    seed_prompts: List[str] = field(default_factory=lambda: SEED_PROMPTS)
    target_topic: str = "token cost governance in AI gateways"
    log_frequency: int = 1


@dataclass
class GenerationReport:
    generation: int
    best_fitness: float
    avg_fitness: float
    total_prompt_tokens: int
    total_completion_tokens: int
    estimated_cost_usd: float
    elapsed_seconds: float
    best_prompt: str


class EvolveOrchestrator:
    def __init__(self, config: EvolveConfig):
        self.config = config
        self.population: List[str] = []
        self.fitness_scores: List[float] = []
        self.results: List[FitnessResult] = []
        self.history: List[GenerationReport] = []
        self.rng = random.Random()

    def seed_population(self):
        seeds = list(self.config.seed_prompts)
        while len(seeds) < self.config.population_size:
            parent = self.rng.choice(seeds)
            variant = mutate_substitute(parent, rate=0.3)
            variant = mutate_insert(variant, rate=0.2)
            seeds.append(variant)
        self.population = seeds[: self.config.population_size]

    def evaluate(self, evaluator: EvaluationLoop):
        self.results = evaluator.evaluate(self.population)
        self.fitness_scores = [r.fitness for r in self.results]
        indexed = list(enumerate(self.fitness_scores))
        indexed.sort(key=lambda x: x[1], reverse=True)
        self.population = [self.population[i] for i, _ in indexed]
        self.results = [self.results[i] for i, _ in indexed]
        self.fitness_scores = [s for _, s in indexed]

    def select_parents(self) -> List[Tuple[str, str]]:
        pairs = []
        num_pairs = self.config.population_size // 2
        for _ in range(num_pairs):
            a = self._tournament_select()
            b = self._tournament_select()
            pairs.append((a, b))
        return pairs

    def _tournament_select(self) -> str:
        contenders = self.rng.sample(
            list(zip(self.population, self.fitness_scores)),
            min(self.config.tournament_size, len(self.population)),
        )
        contenders.sort(key=lambda x: x[1], reverse=True)
        return contenders[0][0]

    def evolve_generation(self):
        elite_count = max(1, int(self.config.population_size * self.config.elite_ratio))
        next_pop = list(self.population[:elite_count])
        pairs = self.select_parents()
        self.rng.shuffle(pairs)
        for parent_a, parent_b in pairs:
            if len(next_pop) >= self.config.population_size:
                break
            child_a, child_b = parent_a, parent_b
            if self.rng.random() < self.config.crossover_rate:
                child_a, child_b = crossover_single_point(parent_a, parent_b)
            for child in [child_a, child_b]:
                if len(next_pop) >= self.config.population_size:
                    break
                child = mutate_substitute(child, rate=self.config.mutation_rate)
                child = mutate_insert(child, rate=self.config.mutation_rate * 0.5)
                child = mutate_delete(child, rate=self.config.mutation_rate * 0.3)
                child = mutate_shuffle(child, rate=self.config.mutation_rate * 0.5)
                next_pop.append(child)
        while len(next_pop) < self.config.population_size:
            next_pop.append(self.rng.choice(self.config.seed_prompts))
        self.population = next_pop[: self.config.population_size]

    def _estimate_cost(self, results: List[FitnessResult]) -> float:
        total_input = sum(r.prompt_tokens for r in results)
        total_output = sum(r.completion_tokens for r in results)
        return total_input * COST_PER_INPUT_TOKEN + total_output * COST_PER_OUTPUT_TOKEN

    def run(self, evaluator: EvaluationLoop) -> List[GenerationReport]:
        self.seed_population()

        for gen in range(self.config.generations):
            start = time.time()
            self.evaluate(evaluator)
            elapsed = time.time() - start
            total_input = sum(r.prompt_tokens for r in self.results)
            total_output = sum(r.completion_tokens for r in self.results)
            cost = self._estimate_cost(self.results)
            best_fitness = max(self.fitness_scores)
            avg_fitness = sum(self.fitness_scores) / len(self.fitness_scores)
            best_idx = self.fitness_scores.index(best_fitness)

            report = GenerationReport(
                generation=gen + 1,
                best_fitness=round(best_fitness, 4),
                avg_fitness=round(avg_fitness, 4),
                total_prompt_tokens=total_input,
                total_completion_tokens=total_output,
                estimated_cost_usd=round(cost, 6),
                elapsed_seconds=round(elapsed, 2),
                best_prompt=self.population[best_idx],
            )
            self.history.append(report)

            if (gen + 1) % self.config.log_frequency == 0:
                logger.info(
                    f"Gen {gen+1:2d} | best={report.best_fitness:.4f} "
                    f"avg={report.avg_fitness:.4f} tokens={total_input+total_output} "
                    f"cost=${report.estimated_cost_usd:.6f} {report.elapsed_seconds:.1f}s"
                )

            if gen < self.config.generations - 1:
                self.evolve_generation()

        return self.history

    def print_final_report(self):
        best = max(self.history, key=lambda r: r.best_fitness)
        final = self.history[-1]
        total_cost = sum(r.estimated_cost_usd for r in self.history)
        total_tokens = sum(r.total_prompt_tokens + r.total_completion_tokens for r in self.history)
        print()
        print("=" * 60)
        print("EVOLUNET-SLM: EVOLUTION COMPLETE")
        print("=" * 60)
        print(f"  Generations:     {len(self.history)}")
        print(f"  Population:      {self.config.population_size}")
        print(f"  Total tokens:    {total_tokens}")
        print(f"  Total cost:      ${total_cost:.6f}")
        print(f"  Best fitness:    {best.best_fitness}")
        print(f"  Final avg fitness: {final.avg_fitness}")
        print()
        print(f"  Best prompt (gen {best.generation}):")
        print(f"    {best.best_prompt}")
        print()
        print("  Generation history:")
        print(f"  {'Gen':>4} {'Best':>8} {'Avg':>8} {'Tokens':>8} {'Cost':>10} {'Time':>6}")
        for r in self.history:
            tokens = r.total_prompt_tokens + r.total_completion_tokens
            print(f"  {r.generation:>4} {r.best_fitness:>8.4f} {r.avg_fitness:>8.4f} {tokens:>8} ${r.estimated_cost_usd:>.6f} {r.elapsed_seconds:>5.1f}s")
        print("=" * 60)


if __name__ == "__main__":
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%H:%M:%S",
    )

    model_cfg = ModelConfig()
    cpu_cfg = CPUConfig()
    infer_cfg = InferenceConfig(max_tokens=128)

    loader = HFModelLoader(model_cfg, cpu_cfg, infer_cfg)
    evaluator = EvaluationLoop(loader)

    config = EvolveConfig(
        population_size=8,
        generations=5,
        mutation_rate=0.15,
    )

    orchestrator = EvolveOrchestrator(config)
    history = orchestrator.run(evaluator)
    orchestrator.print_final_report()
