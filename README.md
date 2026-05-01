# ZUKT — Semantic Search for Claude Code

> **Local-first, hybrid search. Lexical precision meets vector semantics. Your codebase finally understands context.**

---

## The Problem

Claude Code can grep. It finds exact strings with superhuman speed. But when you ask *"where's the authentication middleware?"* or *"show me all error handling patterns"* — it struggles.

**Why?** Because `grep` is literal. It doesn't understand that:
- `authenticate()`, `verify_user()`, and `check_auth()` are conceptually the same
- Error handling in a try/catch is the same pattern as in an if-guard
- A 50K line monorepo is too much noise for naive keyword matching

You end up drowning in false positives, or you manually hunt through files.

**Cursor** solved this with native semantic search. **But Cursor costs money, locks you into their editor, and sends your code to their servers.**

ZUKT fixes this. Locally. Privately. And it integrates with Claude Code via MCP.

---

## What ZUKT Does

ZUKT is a **Model Context Protocol (MCP) server** that adds three search modes to Claude Code:

### **Lexical Search** (Zoekt)
Find exact patterns, regex, symbol definitions. Fast. Precise. Respects `.gitignore`.

```bash
sym:User          # Find the User class definition
r:^async\s+fn    # Find all async functions
file:*.test.ts   # Search only test files
lang:go          # Search only Go files
```

### **Semantic Search** (Vector embeddings)
Find code by *meaning*, not keywords.

```
"Where do we validate user input?"
"Show me all database queries"
"Find retry logic"
```

The neural network understands context. You get relevant results even when variable names don't match.

### **Hybrid Search** (RRF fusion)
Combine both. Lexical narrows the noise, semantic finds the intent.

- Lexical finds 847 potential matches for `error`
- Semantic filters to the 12 that are *actually* about error handling
- You get the best of both worlds

---

## Architecture

```
Your IDE / Claude Code
        ↓
    MCP Protocol (stdio)
        ↓
    npx zukt-mcp          ← One command. Manages everything.
        ├─ Qdrant         ← Vector database (Docker)
        ├─ Ollama         ← Local LLM embeddings (no API keys)
        └─ Zoekt          ← Lexical indexing (provided)
```

**Zero cloud. Zero lock-in. 100% runs on your machine.**

### What's Included
- **Zoekt wrapper** — Fast, regex-aware code search (Go backend)
- **Qdrant integration** — Scalable vector DB for embeddings
- **Ollama bridge** — Use any local embedding model (no costs)
- **npm wrapper** — Easy setup: `npx zukt-mcp` starts everything
- **Docker Compose** — One file, launch Qdrant in seconds

---

## Why This Matters

### **For individual developers:**
- Spend less time navigating unfamiliar codebases
- Understand code patterns without reading 10 files
- Faster PR reviews and context-switching

### **For teams:**
- Onboard engineers faster. Their first month doesn't start in the docs; it starts searching your code *directly*
- Reduce duplicate logic. Find where similar problems were solved before
- Better code discovery than "ask the person who wrote it"

### **For enterprises:**
- Compliance: Your code stays on your machine. No data leakage to APIs
- Cost: No per-search charges, no embedding API subscriptions
- Flexibility: Swap embedding models (Ollama → Llama2, Mistral, Nomic, whatever)

---

## Quick Start

### Prerequisites
- Claude Code CLI installed
- Docker + Docker Compose (for Qdrant)
- 8GB+ RAM (Qdrant + Ollama together)

### 1. Start the infrastructure

```bash
docker-compose -f npm/zukt-mcp/docker/docker-compose.yml up -d
```

This starts Qdrant and pulls the Ollama embedding model in the background.

### 2. Launch ZUKT

```bash
npx -y zukt-mcp
```

It will:
- Verify Zoekt is available
- Connect to Qdrant
- Expose MCP tools to Claude Code
- Print connection details

### 3. Configure Claude Code

Add to your `clauderc.json` (or the editor's MCP config):

```json
{
  "mcpServers": {
    "zukt": {
      "command": "npx",
      "args": ["-y", "zukt-mcp"],
      "env": {
        "ZOEKT_ALLOWED_ROOTS": "/home/your-user",
        "ZUKT_SEMANTIC_BACKEND": "qdrant"
      }
    }
  }
}
```

### 4. Index your codebase

```bash
zukt-mcp prepare --repo /path/to/your/project
```

This chunks, embeds, and indexes everything. First run takes a few minutes depending on codebase size. Subsequent runs are incremental.

### 5. Search

In Claude Code, use the new tools:

```
"search_code" with:
  query: "authentication middleware"
  mode: "hybrid"
  limit: 10
```

Get back the 10 most relevant chunks from your entire codebase.

---

## Commands & Tools

### MCP Tools (exposed to Claude Code)

**`search_code`** — Your main query interface
- `query` (required): Natural language or Zoekt regex
- `mode` (optional): `lexical` | `semantic` | `hybrid` (default: `hybrid`)
- `repo` (optional): Target a specific repository
- `limit` (optional): Max results (default: 10)
- `verbosity` (optional): `brief` | `full` (show code context)

**`prepare_semantic_index`** — Index a repo for vector search
- `repo`: Path or name to index

**`list_repos`** — See all indexed repositories

**`get_status`** — Health check (Zoekt? Qdrant? Embedding API?)

**`get_file`** — Fetch a file or specific lines

**`get_context`** — Get surrounding context for a result

### CLI Commands

```bash
zukt-mcp run                # Start the MCP server

zukt-mcp doctor             # Health check (diagnose issues)

zukt-mcp setup              # Interactive configuration

zukt-mcp prepare --repo <path>   # Index a codebase

zukt-mcp search --query "pattern" [--repo <path>]  # Direct search

zukt-mcp config             # Show current configuration
```

---

## Configuration

### Environment Variables

#### Zoekt Backend
```bash
ZOEKT_BACKEND=http                          # Only HTTP supported currently
ZOEKT_HTTP_BASE_URL=http://127.0.0.1:6070  # Zoekt server address
ZOEKT_HTTP_TIMEOUT=5s                        # Request timeout
ZOEKT_ALLOWED_ROOTS=/home/user               # Which dirs to search
ZOEKT_EXCLUDE_DIRS=.git,node_modules,.venv  # Patterns to skip
```

#### Semantic Backend
```bash
ZUKT_SEMANTIC_BACKEND=qdrant  # Options: disabled | hash | qdrant
```

#### Qdrant (Vector Database)
```bash
ZUKT_QDRANT_URL=http://127.0.0.1:6333
ZUKT_QDRANT_COLLECTION_PREFIX=zukt
ZUKT_QDRANT_TIMEOUT=10s
```

#### Embeddings (Ollama)
```bash
ZUKT_EMBEDDING_PROVIDER=ollama
ZUKT_OLLAMA_URL=http://127.0.0.1:11434
ZUKT_EMBEDDING_MODEL=nomic-embed-text      # 768 dims, fast, accurate
ZUKT_EMBEDDING_DIM=768
ZUKT_EMBEDDING_TIMEOUT=30s
```

#### Chunking
```bash
ZUKT_CHUNK_LINES=80         # Lines per chunk
ZUKT_CHUNK_OVERLAP=20       # Overlap between chunks
ZUKT_MAX_FILE_BYTES=1048576 # Skip files >1MB
```

#### Advanced
```bash
ZUKT_LOG_LEVEL=info                # debug | info | warn | error
ZUKT_CACHE_DIR=/tmp/zukt-cache     # Cache embeddings locally
ZUKT_RRF_K=60                       # Reciprocal Rank Fusion parameter
```

---

## How It Works Under the Hood

### Indexing Phase

1. **Scan** — Walk the repo, respect `.gitignore`
2. **Chunk** — Split files into ~80-line overlapping chunks
3. **Embed** — Send chunks to Ollama, get 768-dim vectors
4. **Store** — Index in Qdrant for fast similarity search

All steps are incremental. Re-index only changed files.

### Search Phase (Hybrid)

```
User: "where is token validation?"

Step 1: Lexical (Zoekt)
├─ Search: r:validate|token|check
├─ Get: 847 matches (too many!)
└─ Score by relevance

Step 2: Semantic (Qdrant)
├─ Embed the query: "where is token validation?"
├─ Find 100 most similar chunks
└─ Rank by vector similarity

Step 3: Fusion (RRF)
├─ Combine rank lists
├─ Remove duplicates
├─ Return top 10
```

**Result:** Lexical noise is filtered by semantic understanding.

---

## Real Examples

### Example 1: Finding Error Handlers

**Query:** `"where do we catch errors?"`

**Lexical alone:** 1,200+ matches (every `except`, `catch`, `try`)

**Semantic alone:** 40 matches (mostly good, some false positives)

**Hybrid:** 15 results, all relevant error handling patterns

---

### Example 2: Understanding a Protocol

You're new. You need to understand how auth tokens flow through your system.

**Query:** `"how does token refresh work?"`

**Old way (without ZUKT):** 
- Grep for `refresh_token`
- Read 8 files
- Grep for `authenticate`
- Read 5 more files
- Piece together the flow manually
- **Time: 30 minutes**

**With ZUKT:**
- One search: `"how does token refresh work?"`
- Get the 5 most relevant chunks from across your codebase
- Read them in context
- Understand the flow
- **Time: 3 minutes**

---

### Example 3: Finding Similar Implementations

**Query:** `"retry logic"` (semantic search)

Results:
- Exponential backoff in HTTP client
- Retry loop in database connection
- Retry wrapper around API calls

All found in seconds, without knowing their exact names.

---

## Performance

### Benchmarks (on a ~500K LOC codebase)

| Operation | Time |
|-----------|------|
| Index new repo (cold) | ~8 min (first time, with Ollama) |
| Re-index (incremental) | ~30 sec |
| Lexical search | 50–150ms |
| Semantic search | 200–400ms |
| Hybrid (lexical + semantic) | 300–500ms |

Memory: Qdrant ≈ 600MB, Zoekt ≈ 400MB, Ollama ≈ 1.2GB

---

## Comparison

| Feature | ZUKT | Cursor | Claude Code + MCP (other) |
|---------|------|--------|---------------------------|
| **Semantic search** | ✓ | ✓ | Varies |
| **Local-first** | ✓ | ✗ (cloud) | ✓ (usually) |
| **Lexical precision** | ✓ (Zoekt) | ✓ | ✓ (grep) |
| **Configurable backend** | ✓ | ✗ | ✗ |
| **Cost** | Free | $20/mo | Free–$$ |
| **Data privacy** | Your machine | Sent to Cursor | Your machine |
| **Setup time** | 5 min | Install | 10 min |

---

## Architecture Decisions

### Why Zoekt?

Zoekt is *fast*, *smart*, and *respects .gitignore*. It's what Google uses internally for code search. Alternatives like `ripgrep` or `grep` are okay, but Zoekt's structured search (`sym:`, `lang:`, `file:`) is irreplaceable for dense codebases.

### Why Qdrant?

Qdrant is production-grade, lightweight, and runs locally in Docker. Alternatives (Pinecone, Weaviate) are cloud-only. Qdrant is local-first.

### Why Ollama?

Ollama runs embedding models locally without API costs. You control which model (Nomic Embed, Mistral, etc.) and never leak code to external services.

### Why RRF (Reciprocal Rank Fusion)?

RRF is a battle-tested algorithm for combining multiple ranked lists (lexical + semantic). It's simple, deterministic, and avoids the problem of "how do I score a lexical result vs. a semantic result?"

---

## Roadmap

### Near-term (next 2 months)
- [ ] GPU acceleration for embeddings (CUDA/Metal)
- [ ] Multi-repository indexing with shared Qdrant collections
- [ ] Web UI for index management + visualization
- [ ] Incremental chunk re-embedding (only changed lines)

### Medium-term (next 6 months)
- [ ] Fine-tuned embedding model for code (better domain-specific accuracy)
- [ ] AST-aware chunking (split at function/class boundaries, not lines)
- [ ] Call-graph indexing (find who calls function X across repos)
- [ ] Cross-language semantic search (same logic across Rust/Python/Go)

### Long-term
- [ ] IDE plugins (VS Code, JetBrains native integration)
- [ ] Team collaboration (shared index server with auth)
- [ ] Marketplace for pre-built indexes (index popular libs once, reuse)

---

## Contributing

ZUKT is open source and welcomes contributions.

### Areas we need help with:
- **Performance:** Optimize embedding generation, Qdrant queries
- **Integrations:** Support more embedding providers (HuggingFace, Together, etc.)
- **Testing:** Expand test coverage and add benchmark suites
- **Documentation:** Improve guides for non-standard setups
- **Community:** Share your use cases, integrations, improvements

See [CONTRIBUTING.md](./CONTRIBUTING.md) for details.

---

## Troubleshooting

### `zukt-mcp` won't start

Run `zukt-mcp doctor` to diagnose:
```bash
zukt-mcp doctor
```

This checks:
- Zoekt availability (HTTP endpoint)
- Qdrant connectivity
- Ollama status
- Disk permissions

### Searches are slow

1. Check if Qdrant is indexing: `zukt-mcp doctor`
2. First search after indexing is always slow (Qdrant warm-up)
3. Try `--limit 5` to reduce results to fetch
4. Increase `ZUKT_QDRANT_TIMEOUT` if indexing large repos

### Out of memory

Reduce `ZUKT_EMBEDDING_DIM` or use a smaller model:
```bash
ZUKT_EMBEDDING_MODEL=minilm      # 384 dims, lighter
ZUKT_EMBEDDING_DIM=384
```

### High CPU usage during indexing

This is normal. Ollama and Qdrant both use CPU heavily. They'll settle after indexing completes.

---

## License

MIT. Use it freely. Modify it. Sell products on top of it (just give credit).

---

## Acknowledgments

- [Zoekt](https://github.com/google/zoekt) (Google's code search engine)
- [Qdrant](https://qdrant.tech) (vector database)
- [Ollama](https://ollama.ai) (local LLM inference)
- [MCP](https://modelcontextprotocol.io) (Anthropic's protocol for tool integration)

---

## Get Started Now

```bash
# Clone or download ZUKT
git clone https://github.com/404loopback/ZUKT
cd ZUKT

# Start infrastructure
docker-compose -f npm/zukt-mcp/docker/docker-compose.yml up -d

# Launch MCP server
npx -y zukt-mcp

# Index your first repo
zukt-mcp prepare --repo /path/to/your/project

# Configure Claude Code and start searching!
```

**Questions?** Open an issue or check the [docs](./docs).

---

**ZUKT: Search your code like you understand it.**
