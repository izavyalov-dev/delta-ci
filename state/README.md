# state

Persistence layer for authoritative run, job, attempt, and lease state.
This package owns migrations, schema definitions, and enforcement of the state
machine constraints described in `docs/architecture/state-machines.md`.
