# Codex Routing Guide for ZUKT

Use `search_code` before opening files broadly.

## Recommended mode routing

- `lexical`
  - Exact symbols and identifiers
  - Stack traces
  - Filenames
  - Config keys
  - Literal strings

- `semantic`
  - Unknown naming
  - Conceptual or behavior-based query
  - Cross-module intent queries

- `hybrid`
  - General investigations
  - Feature walkthroughs
  - Broad debugging where both exact and conceptual signals matter

## Workflow

1. Run `search_code` with the most suitable mode.
2. Validate findings with `get_context` or `get_file`.
3. Edit only after source verification.

## Anti-patterns

- Editing from search snippets only.
- Re-running near-identical semantic queries repeatedly.
- Using `semantic` for pure exact lookups (prefer `lexical`).
