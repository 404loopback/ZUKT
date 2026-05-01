# AGENTS.md

## Engineering policy

Always implement the best technical solutions. Avoid quick fixes and workarounds; prefer clean and stable code.

## Search policy for Codex

Use the MCP tool `search_code` before broad code exploration.

Use `mode: "hybrid"` when searching by behavior, feature, responsibility, UI flow, validation logic, business rule, or vague natural language.

Use `mode: "semantic"` when lexical terms are unknown or misleading.

Use `mode: "lexical"` for exact identifiers, stack traces, filenames, symbols, function names, config keys, or literal strings.

After search results, call `get_context` or `get_file` to verify source code before editing.

Do not edit based only on snippets returned by `search_code`.

Do not call semantic search repeatedly with near-identical queries. If results are poor, rewrite the query using domain terms, file names, or symbols discovered so far.
