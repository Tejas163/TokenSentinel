# EvoluNet-SLM — Evolutionary Prompt Optimization Sandbox

CPU-only evolutionary prompt engine using GGUF quantized SLMs.

## Directory Layout

```
evolunet_slm/
├── requirements.txt         # llama-cpp-python, numpy, huggingface_hub
├── config.py                # CPUConfig, InferenceConfig, ModelConfig dataclasses
├── fitness_engine.py        # GGUFModelLoader — CPU-optimized load + basic generation
├── mutation_operator.py     # Prompt crossover + mutation function stubs
├── evolve.py                # EvolveOrchestrator skeleton (not yet implemented)
├── download_model.py        # Fetches Qwen2-0.5B-Instruct Q4_K_M GGUF → models/
├── models/                  # Local GGUF model storage
└── AGENTS.md                # this file
```

## Data Flow

```
download_model.py → models/*.gguf → GGUFModelLoader.load() → generate()
```
