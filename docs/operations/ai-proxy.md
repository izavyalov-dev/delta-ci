# AI Proxy

This document describes the optional **AI proxy** used to connect Delta CI to
OpenAI without embedding provider-specific logic in the orchestrator.

The proxy accepts a **generic explanation request** and returns a short,
advisory summary.

---

## Why a Proxy?

Delta CI keeps AI provider logic out of the control plane. The proxy:
- isolates provider credentials
- enforces request timeouts
- keeps AI calls off the orchestrator process

---

## API Contract

Endpoint:
```
POST /v1/explain
```

Request:
```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "prompt_version": "failure-explain-v1",
  "prompt": "..."
}
```

Response:
```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "summary": "Short advisory explanation.",
  "details": ""
}
```

`summary` is required. `details` is optional.

---

## Running the Proxy

```sh
OPENAI_API_KEY=... \
go run ./cmd/ai-proxy \
  -listen :8090 \
  -openai-model gpt-4o-mini
```

Orchestrator config:
```sh
DELTA_AI_ENABLED=true \
DELTA_AI_ENDPOINT=http://localhost:8090/v1/explain \
DELTA_AI_PROVIDER=openai \
DELTA_AI_MODEL=gpt-4o-mini \
go run ./cmd/orchestrator serve -listen :8080
```

---

## Security Notes

- The proxy should be deployed in a restricted network segment.
- Do not log raw prompts or AI responses.
- Treat AI output as advisory only.
