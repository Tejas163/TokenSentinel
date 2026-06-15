"""FastAPI HTTP service wrapping EvoluNet-SLM orchestrator."""

import logging
import uuid
import os
from contextlib import asynccontextmanager
from threading import Thread
from typing import Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from config import CPUConfig, InferenceConfig, ModelConfig
from evolve import EvolveConfig, EvolveOrchestrator, GenerationReport
from fitness_engine import EvaluationLoop, HFModelLoader

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)


class EvolveRequest(BaseModel):
    population_size: int = 8
    generations: int = 5
    mutation_rate: float = 0.15
    crossover_rate: float = 0.7
    elite_ratio: float = 0.1
    tournament_size: int = 3
    target_topic: str = "token cost governance in AI gateways"
    max_tokens: int = 128


class TaskStatus(BaseModel):
    task_id: str
    status: str
    progress: Optional[str] = None


class TaskResult(BaseModel):
    task_id: str
    status: str
    history: list[GenerationReport]
    final_report: str


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool


task_store: dict[str, dict] = {}
loader: Optional[HFModelLoader] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global loader
    model_cfg = ModelConfig()
    cpu_cfg = CPUConfig()
    infer_cfg = InferenceConfig(max_tokens=128)
    loader = HFModelLoader(model_cfg, cpu_cfg, infer_cfg)
    try:
        loader.load()
        logger.info("Model loaded on startup")
    except FileNotFoundError as e:
        logger.warning("Model not found at %s: %s", model_cfg.model_dir, e)
        logger.warning("Run python download_model.py first, or mount a pre-downloaded model")
        loader = None
    yield
    if loader:
        loader.unload()


app = FastAPI(title="EvoluNet-SLM", version="0.1.0", lifespan=lifespan)


def _run_evolution(task_id: str, req: EvolveRequest):
    global loader
    task_store[task_id]["status"] = "running"
    try:
        if loader is None:
            task_store[task_id]["status"] = "failed"
            task_store[task_id]["error"] = "Model not loaded. Run download_model.py first."
            return

        evaluator = EvaluationLoop(loader)
        config = EvolveConfig(
            population_size=req.population_size,
            generations=req.generations,
            mutation_rate=req.mutation_rate,
            crossover_rate=req.crossover_rate,
            elite_ratio=req.elite_ratio,
            tournament_size=req.tournament_size,
            target_topic=req.target_topic,
        )
        orchestrator = EvolveOrchestrator(config)
        history = orchestrator.run(evaluator)
        task_store[task_id]["status"] = "completed"
        task_store[task_id]["history"] = history
        task_store[task_id]["orchestrator"] = orchestrator
    except Exception as e:
        logger.exception("Evolution failed")
        task_store[task_id]["status"] = "failed"
        task_store[task_id]["error"] = str(e)


@app.post("/evolve", response_model=TaskStatus)
async def start_evolution(req: EvolveRequest):
    task_id = str(uuid.uuid4())
    task_store[task_id] = {"status": "pending", "request": req}
    thread = Thread(target=_run_evolution, args=(task_id, req), daemon=True)
    thread.start()
    return TaskStatus(task_id=task_id, status="pending")


@app.get("/evolve/{task_id}/status", response_model=TaskStatus)
async def get_status(task_id: str):
    task = task_store.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    return TaskStatus(task_id=task_id, status=task["status"])


@app.get("/evolve/{task_id}/results", response_model=TaskResult)
async def get_results(task_id: str):
    task = task_store.get(task_id)
    if task is None:
        raise HTTPException(status_code=404, detail="Task not found")
    if task["status"] == "running":
        raise HTTPException(status_code=400, detail="Task still running")
    if task["status"] == "failed":
        return TaskResult(
            task_id=task_id,
            status="failed",
            history=[],
            final_report=f"Evolution failed: {task.get('error', 'unknown error')}",
        )
    orch = task.get("orchestrator")
    if orch:
        import io, sys
        buf = io.StringIO()
        old_stdout = sys.stdout
        sys.stdout = buf
        try:
            orch.print_final_report()
        finally:
            sys.stdout = old_stdout
        final_report = buf.getvalue()
    else:
        final_report = "No report available"
    return TaskResult(
        task_id=task_id,
        status="completed",
        history=task.get("history", []),
        final_report=final_report,
    )


@app.get("/health", response_model=HealthResponse)
async def health():
    return HealthResponse(
        status="ok",
        model_loaded=loader is not None,
    )


if __name__ == "__main__":
    import uvicorn
    port = int(os.getenv("EVOLUNET_PORT", "8000"))
    uvicorn.run(app, host="0.0.0.0", port=port)
