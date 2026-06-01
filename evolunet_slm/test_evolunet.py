"""Tests for EvoluNet-SLM mutation operators and orchestrator."""

import random
from unittest.mock import MagicMock

from mutation_operator import (
    crossover_single_point,
    crossover_uniform,
    mutate_substitute,
    mutate_insert,
    mutate_delete,
    mutate_shuffle,
)
from evolve import EvolveOrchestrator, EvolveConfig


# --- Mutation Operator Tests ---


class TestCrossoverSinglePoint:
    def test_returns_two_children(self):
        a, b = "explain the concept simply", "list advantages and disadvantages"
        c1, c2 = crossover_single_point(a, b)
        assert isinstance(c1, str)
        assert isinstance(c2, str)
        assert len(c1) > 0
        assert len(c2) > 0

    def test_children_differ_from_parents(self):
        a, b = "explain step by step", "list every detail"
        c1, c2 = crossover_single_point(a, b)
        total = {a, b, c1, c2}
        assert len(total) >= 3, "children should differ from at least one parent"

    def test_preserves_all_tokens(self):
        a, b = "hello world test", "foo bar baz"
        c1, c2 = crossover_single_point(a, b)
        all_tokens = c1.split() + c2.split()
        assert set(all_tokens) == {"hello", "world", "test", "foo", "bar", "baz"}

    def test_short_strings_returned_as_is(self):
        c1, c2 = crossover_single_point("a", "b")
        assert c1 == "a"
        assert c2 == "b"


class TestCrossoverUniform:
    def test_returns_two_children(self):
        c1, c2 = crossover_uniform("a b c", "d e f")
        assert isinstance(c1, str)
        assert isinstance(c2, str)

    def test_children_not_identical_to_parents(self):
        random.seed(42)
        a, b = "one two three four", "five six seven eight"
        c1, c2 = crossover_uniform(a, b, prob=0.5)
        assert c1 != a or c2 != b


class TestMutateSubstitute:
    def test_replaces_known_synonyms(self):
        result = mutate_substitute("explain the concept", rate=1.0)
        assert result != "explain the concept"

    def test_unknown_words_unchanged(self):
        result = mutate_substitute("xyzzy flurbo garblex", rate=1.0)
        assert result == "xyzzy flurbo garblex"

    def test_rate_zero_no_change(self):
        result = mutate_substitute("explain the concept", rate=0.0)
        assert result == "explain the concept"

    def test_preserves_punctuation_attachment(self):
        result = mutate_substitute("explain good!", rate=1.0)
        assert "!" in result


class TestMutateInsert:
    def test_inserts_instructional_phrase(self):
        result = mutate_insert("explain this", rate=1.0)
        assert len(result.split()) > 2

    def test_rate_zero_no_change(self):
        result = mutate_insert("explain this", rate=0.0)
        assert result == "explain this"

    def test_empty_string_returns_empty(self):
        result = mutate_insert("", rate=1.0)
        assert result == ""


class TestMutateDelete:
    def test_removes_some_tokens(self):
        random.seed(42)
        phrase = "one two three four five six seven eight nine ten"
        result = mutate_delete(phrase, rate=0.5)
        assert len(result.split()) < len(phrase.split())

    def test_rate_zero_no_change(self):
        result = mutate_delete("one two three four five", rate=0.0)
        assert result == "one two three four five"

    def test_short_strings_preserved(self):
        result = mutate_delete("a b", rate=1.0)
        assert result == "a b"


class TestMutateShuffle:
    def test_reorders_tokens(self):
        random.seed(42)
        phrase = "one two three four five six seven eight"
        result = mutate_shuffle(phrase, rate=0.3)
        assert result.split() != phrase.split()

    def test_rate_zero_no_change(self):
        result = mutate_shuffle("one two three four", rate=0.0)
        assert result == "one two three four"

    def test_single_token_unchanged(self):
        result = mutate_shuffle("hello", rate=1.0)
        assert result == "hello"


# --- Fitness Scorer Tests ---


def _make_mock_fitness_result(prompt="test", fitness=0.5, prompt_tokens=10, completion_tokens=20):
    """Helper to create mock FitnessResult objects."""
    from fitness_engine import FitnessResult
    return FitnessResult(
        prompt=prompt, response="ok", fitness=fitness,
        prompt_tokens=prompt_tokens, completion_tokens=completion_tokens,
        latency_ms=5.0, response_length=2,
    )


class TestDefaultFitnessScorer:
    def test_returns_float(self):
        from fitness_engine import default_fitness_scorer
        score = default_fitness_scorer("some response text", 10, 20)
        assert isinstance(score, float)

    def test_longer_responses_score_higher(self):
        from fitness_engine import default_fitness_scorer
        short = default_fitness_scorer("short", 5, 10)
        long = default_fitness_scorer("one two three four five six seven eight nine ten", 5, 10)
        assert long > short

    def test_score_between_0_and_1(self):
        from fitness_engine import default_fitness_scorer
        score = default_fitness_scorer("a b c d e" * 20, 50, 100)
        assert 0.0 < score < 1.5


# --- Orchestrator Tests ---


class TestEvolveOrchestrator:
    def test_seed_population_creates_correct_size(self):
        config = EvolveConfig(population_size=10)
        orch = EvolveOrchestrator(config)
        orch.seed_population()
        assert len(orch.population) == 10

    def test_seed_population_uses_seeds(self):
        config = EvolveConfig(seed_prompts=["hello world"])
        orch = EvolveOrchestrator(config)
        orch.seed_population()
        assert any("hello" in p for p in orch.population)
        assert any("world" in p for p in orch.population)

    def test_tournament_select_returns_from_population(self):
        config = EvolveConfig(population_size=10)
        orch = EvolveOrchestrator(config)
        orch.seed_population()
        orch.fitness_scores = [random.random() for _ in range(10)]
        selected = orch._tournament_select()
        assert selected in orch.population

    def test_evolve_generation_maintains_size(self):
        config = EvolveConfig(population_size=12)
        orch = EvolveOrchestrator(config)
        orch.seed_population()
        orch.fitness_scores = [random.random() for _ in range(12)]
        orch.evolve_generation()
        assert len(orch.population) == 12

    def test_select_parents_returns_pairs(self):
        config = EvolveConfig(population_size=10)
        orch = EvolveOrchestrator(config)
        orch.seed_population()
        orch.fitness_scores = [random.random() for _ in range(10)]
        pairs = orch.select_parents()
        assert len(pairs) == 5
        for a, b in pairs:
            assert isinstance(a, str)
            assert isinstance(b, str)

    def test_run_with_mock_evaluator(self):
        from fitness_engine import FitnessResult
        config = EvolveConfig(population_size=6, generations=3)
        orch = EvolveOrchestrator(config)

        mock_eval = MagicMock()
        mock_eval.evaluate.return_value = [
            FitnessResult(prompt=f"prompt_{i}", response="ok", fitness=random.random(),
                          prompt_tokens=10, completion_tokens=20, latency_ms=5.0,
                          response_length=2)
            for i in range(config.population_size)
        ]

        history = orch.run(mock_eval)
        assert len(history) == 3
        assert all(isinstance(r.best_fitness, float) for r in history)
        assert all(r.estimated_cost_usd > 0 for r in history)

    def test_cost_tracking_adds_up(self):
        from fitness_engine import FitnessResult
        config = EvolveConfig(population_size=4, generations=2)
        orch = EvolveOrchestrator(config)

        mock_eval = MagicMock()
        mock_eval.evaluate.return_value = [
            FitnessResult(prompt=f"p{i}", response="ok", fitness=0.5,
                          prompt_tokens=20, completion_tokens=30, latency_ms=5.0,
                          response_length=2)
            for i in range(config.population_size)
        ]

        history = orch.run(mock_eval)
        gen_cost = 4 * (20 * 0.0000015 + 30 * 0.0000020)
        total_expected = round(gen_cost * 2, 6)
        total_actual = round(sum(r.estimated_cost_usd for r in history), 6)
        assert total_actual == total_expected, f"{total_actual} != {total_expected}"


# --- Integration: config loads correctly ---


class TestConfig:
    def test_evolve_config_defaults(self):
        c = EvolveConfig()
        assert c.population_size == 20
        assert c.generations == 10
        assert 0 < c.mutation_rate < 1
        assert len(c.seed_prompts) == 8

    def test_evolve_config_custom(self):
        c = EvolveConfig(population_size=4, generations=2)
        assert c.population_size == 4
        assert c.generations == 2
