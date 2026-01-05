# cmd

Command entrypoints for Delta CI binaries.

- `cmd/orchestrator` wires up the control-plane API and orchestrator service.
- `cmd/runner` provides the minimal runner CLI used by the data plane.

Control-plane and runner logic live in sibling packages at the repository root.
