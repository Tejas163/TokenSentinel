# EvoluNet-SLM — Evolutionary Prompt Optimization

CPU-only evolutionary prompt engine using HuggingFace transformers (Qwen2-0.5B-Instruct).

## Directory Layout

```
evolunet_slm/
├── requirements.txt         # transformers, torch, numpy, huggingface_hub
├── config.py                # CPUConfig, InferenceConfig, ModelConfig dataclasses
├── fitness_engine.py        # HFModelLoader (transformers + torch), EvaluationLoop, fitness scoring
├── mutation_operator.py     # 5 mutation operators + 2 crossover methods
├── evolve.py                # EvolveOrchestrator — full GA loop with cost tracking
├── download_model.py        # Fetches Qwen2-0.5B-Instruct safetensors → models/
├── models/                  # Local model storage (safetensors + GGUF cache)
└── test_evolunet.py         # 31 tests (mutation, scorer, orchestrator, config)
```

## Data Flow

```
download_model.py → models/qwen2-0_5b-instruct/*.safetensors → HFModelLoader.load() → generate()
```

## Status

- **31/31 tests pass** (unit + mock integration)
- **End-to-end validated** with real Qwen2-0.5B-Instruct inference
- Typical run: 8 prompts × 5 generations ≈ 6 min on CPU, ~$0.006 estimated cost

## Known Limitations

- Fitness function is heuristic (response length + structure bonuses) — not task-grounded
- Population/convergence dynamics are toy-grade for a 500M model
- No persistence of evolved prompts across runs
