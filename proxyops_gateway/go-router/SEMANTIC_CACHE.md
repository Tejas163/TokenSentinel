# Semantic Cache Configuration

## Phase 1: MinHash (Default)

Already enabled with these defaults in `docker-compose.yml`:

```yaml
- SEMANTIC_CACHE_ENABLED=true
- SEMANTIC_CACHE_MODE=minhash
- SEMANTIC_CACHE_TTL=1h
```

Works with standard Redis (`redis:7-alpine`). No additional setup needed.

## Phase 2: Embeddings

Switch from MinHash to embedding-based vector search for higher accuracy:

### 1. Get an Embedding API Key

| Provider | API Key URL | Free Tier |
|----------|------------|-----------|
| OpenAI | https://platform.openai.com/api-keys | $5 trial credit |
| Google Gemini | https://aistudio.google.com/app/apikey | 60 req/min free |
| Local (no key) | Run a local embedding server like https://github.com/Devons/binary-cache-server | Free |

### 2. Update `docker-compose.yml`

```yaml
go-router:
  environment:
    - SEMANTIC_CACHE_ENABLED=true
    - SEMANTIC_CACHE_MODE=embedding      # switch from minhash to embedding
    - EMBEDDING_API_KEY=sk-proj-...       # your API key
    - EMBEDDING_API_URL=https://api.openai.com/v1/embeddings
    - EMBEDDING_MODEL=text-embedding-ada-002
```

### 3. Switch Redis to Redis Stack

Phase 2 requires `redis-stack-server` (has vector search). Change the redis image in `docker-compose.yml`:

```yaml
redis:
  image: redis/redis-stack-server:7.2.0-v11   # was redis:7-alpine
```

### 4. Redeploy

```bash
docker compose up -d --build go-router redis
```

### Verify

Look for this log line:
```json
{"level":"INFO","msg":"semantic cache enabled","mode":"embedding","model":"text-embedding-ada-002"}
```

### Tuning

| Env Var | Default | Description |
|---------|---------|-------------|
| `EMBEDDING_API_URL` | `https://api.openai.com/v1/embeddings` | Any OpenAI-compatible embedding endpoint |
| `EMBEDDING_API_KEY` | — | Bearer token sent as `Authorization: Bearer {key}` |
| `EMBEDDING_MODEL` | `text-embedding-ada-002` | Model name sent in the request body |
| `SEMANTIC_CACHE_THRESHOLD` | `0.85` | Cosine similarity threshold (0.0–1.0); higher = fewer hits but more precise |
| `SEMANTIC_CACHE_TTL` | `1h` | How long cache entries live before expiry |

### Local Embedding Server (No API Key)

Run a local server that serves OpenAI-compatible embeddings:

```bash
docker run -p 9999:9999 devons/binary-cache-server
```

Then set:
```yaml
- EMBEDDING_API_URL=http://host.docker.internal:9999/v1/embeddings
- EMBEDDING_API_KEY=
```
