# runner

Data-plane runner implementations and protocol clients.
Runners execute untrusted workloads under a lease, emit heartbeats, upload logs,
and report completion according to the runner protocol. Phase 0 ships a
minimal CLI runner that executes a single shell command and can upload logs to
AWS S3.
