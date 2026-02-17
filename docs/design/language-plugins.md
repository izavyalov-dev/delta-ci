# Language Plugin System

Delta CI uses a plugin architecture to support multiple programming languages in its diff-aware planner. This document describes the plugin interface, the external plugin protocol, and how to write a new language plugin.

## Overview

The planner discovers which languages are present in a repository, then delegates language-specific work to plugins. Each plugin handles three operations:

1. **Detect** -- Quick check: does this repo use my language?
2. **Discover** -- Full project discovery: find projects, their dependencies, and report relevant file extensions
3. **Plan** -- Given impacted projects, produce build/test/lint jobs

## Built-in Plugins

### Go (`GoLanguagePlugin`)

The Go plugin is compiled into the Delta CI binary and runs in-process. It detects Go projects via `go.mod`, `go.sum`, `go.work`, and `go.work.sum` marker files.

- **Code extensions:** `.go`, `.mod`, `.sum`
- **Global files:** `go.mod`, `go.sum`, `go.work`, `go.work.sum`
- **Jobs produced:** `build` (`go build ./...`), `test` (`go test ./...`), `lint` (`go vet ./...`)

## External Plugin Protocol

External plugins are standalone executables that communicate via JSON over stdin/stdout. They can be written in any language.

### Discovery

Delta CI finds external plugins in two ways:

1. **PATH scanning:** Executables named `delta-ci-lang-<name>` (e.g., `delta-ci-lang-rust`, `delta-ci-lang-python`)
2. **Explicit flag:** `--language-plugin <path>` (repeatable)

### Protocol

The orchestrator sends a single JSON object on stdin and reads a single JSON object from stdout.

#### Request envelope

```json
{
  "action": "detect|discover|plan",
  "version": "1",
  "request": { ... }
}
```

#### Response envelope

```json
{
  "version": "1",
  "error": "",
  "result": { ... }
}
```

If `error` is non-empty, the plugin is considered to have failed. The planner will use a conservative fallback (run everything).

### Actions

#### `detect`

**Request:**
```json
{
  "repo_root": "/path/to/repo"
}
```

**Result:**
```json
{
  "detected": true
}
```

Return `detected: true` if the repository contains files for your language. This should be a fast check (stat a few marker files, do not walk the tree).

#### `discover`

**Request:**
```json
{
  "repo_root": "/path/to/repo"
}
```

**Result:**
```json
{
  "projects": [
    {
      "name": "my-app",
      "root": "apps/my-app",
      "module_path": "example.com/my-app",
      "requires": ["example.com/shared-lib"]
    }
  ],
  "dependency_unknown": false,
  "code_extensions": [".rs", ".toml"],
  "global_files": ["Cargo.toml", "Cargo.lock"],
  "fingerprint_files": ["Cargo.toml", "Cargo.lock"]
}
```

| Field | Description |
|-------|-------------|
| `projects` | List of discovered projects/modules |
| `projects[].name` | Human-readable project name (usually the relative path) |
| `projects[].root` | Relative path from repo root to project directory |
| `projects[].module_path` | Package/module identifier for dependency resolution |
| `projects[].requires` | Module paths this project depends on (for transitive impact) |
| `dependency_unknown` | Set to `true` if the dependency graph could not be fully resolved |
| `code_extensions` | File extensions that represent source code for this language |
| `global_files` | Files that, when changed, should trigger all projects |
| `fingerprint_files` | Files to include in the repo fingerprint hash |

#### `plan`

**Request:**
```json
{
  "repo_root": "/path/to/repo",
  "projects": [
    {"name": "my-app", "root": "apps/my-app", "module_path": "...", "requires": ["..."]}
  ],
  "cache_read_only": false,
  "impact": {
    "docs_only": false,
    "global": true,
    "code_changes": true,
    "paths": ["apps/my-app/src/main.rs"],
    "impacted_projects": ["my-app"],
    "dependency_unknown": false
  }
}
```

**Result:**
```json
{
  "jobs": [
    {
      "name": "build:my-app",
      "required": true,
      "workdir": "apps/my-app",
      "steps": ["cargo build --release"],
      "reason": "code change in my-app",
      "depends_on": []
    },
    {
      "name": "test:my-app",
      "required": true,
      "workdir": "apps/my-app",
      "steps": ["cargo test"],
      "reason": "code change in my-app",
      "depends_on": ["build:my-app"]
    }
  ],
  "skipped_jobs": [
    {
      "name": "lint:my-app",
      "reason": "docs-only change"
    }
  ]
}
```

### Timeout

External plugins have a 30-second timeout by default. If a plugin does not respond within this window, it is treated as a failure and the planner falls back to running all projects.

### Error handling

- If the plugin exits with a non-zero status, it is treated as a failure.
- If the response contains a non-empty `error` field, it is treated as a failure.
- All failures trigger conservative fallback (expand the plan, never skip).

## Writing a Plugin

### Example: Rust plugin (shell script)

```bash
#!/bin/sh
INPUT=$(cat)
ACTION=$(echo "$INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin)['action'])")

case "$ACTION" in
  detect)
    REPO=$(echo "$INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin)['request']['repo_root'])")
    if [ -f "$REPO/Cargo.toml" ]; then
      echo '{"version":"1","result":{"detected":true}}'
    else
      echo '{"version":"1","result":{"detected":false}}'
    fi
    ;;
  discover)
    # ... parse Cargo.toml, find workspace members
    echo '{"version":"1","result":{...}}'
    ;;
  plan)
    # ... generate cargo build/test jobs
    echo '{"version":"1","result":{...}}'
    ;;
esac
```

### Guidelines

1. **Detect should be fast.** Only check for marker files (`Cargo.toml`, `package.json`, etc.). Do not walk the file tree.
2. **Report extensions and globals.** These feed into the language-agnostic impact analysis. Without them, changed files won't be classified correctly.
3. **Honor `impact.docs_only`.** Skip test/lint jobs when only documentation changed.
4. **Use `depends_on` for job ordering.** Test jobs should depend on build jobs.
5. **Never access secrets.** Plugins receive only the repo root path and structural data. The trust model matches git credential helpers.

## Configuration

### Orchestrator flags

```
--language-plugin /path/to/delta-ci-lang-rust    # Explicit plugin path (repeatable)
```

### Automatic discovery

Any executable in `PATH` matching `delta-ci-lang-*` is automatically loaded as an external plugin. The language name is derived from the suffix (e.g., `delta-ci-lang-python` becomes the `python` plugin).

### Default behavior

When no plugins are configured, the built-in Go plugin is used automatically. This preserves backward compatibility with existing Delta CI installations.
