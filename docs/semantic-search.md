# Semantic Search in ZUKT

ZUKT supports three search modes:

- `lexical`: Zoekt exact/DSL search only.
- `semantic`: vector-only search (semantic backend).
- `hybrid`: lexical + semantic fusion using reciprocal rank fusion.

## Backend selection

Configure the semantic backend with:

```env
ZUKT_SEMANTIC_BACKEND=disabled|hash|qdrant
```

- `disabled`: no semantic backend.
- `hash`: in-memory local fallback (no external dependencies).
- `qdrant`: local vector store backed by Qdrant + Ollama embeddings.

## Qdrant + Ollama configuration

```env
ZUKT_SEMANTIC_BACKEND=qdrant

ZUKT_QDRANT_URL=http://127.0.0.1:6333
ZUKT_QDRANT_COLLECTION_PREFIX=zukt

ZUKT_EMBEDDING_PROVIDER=ollama
ZUKT_OLLAMA_URL=http://127.0.0.1:11434
ZUKT_EMBEDDING_MODEL=nomic-embed-text
ZUKT_EMBEDDING_DIM=768

ZUKT_CHUNK_LINES=80
ZUKT_CHUNK_OVERLAP=20
ZUKT_MAX_FILE_BYTES=1048576
```

## Indexation pipeline

`prepare_semantic_index(repo)` runs:

1. Repo scan with local safety rules (`ZOEKT_ALLOWED_ROOTS`, excluded dirs, binary skip, size cap).
2. Chunking (line-window strategy with overlap).
3. Embedding generation.
4. Upsert into semantic backend.

Returned stats include backend/model and indexing counters.

## Search behavior

- `mode=semantic`
  1. Embed query.
  2. Search vector backend.
  3. Return scored file/range hits with `source=vector`.

- `mode=hybrid`
  1. Run lexical pool search.
  2. Run semantic search.
  3. Merge with RRF and deduplicate.

## Fallbacks

- `qdrant` unavailable: service keeps running; semantic quality depends on backend configured.
- `hash` backend remains available for local semantic fallback.
- `disabled` backend keeps lexical mode operational.
