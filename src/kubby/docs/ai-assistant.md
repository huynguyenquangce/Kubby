# AI assistant

← [Architecture skeleton](../ARCHITECTURE.md)

Owns `internal/k8sclient/ai.go` (evidence collection) and `ai.go` in package main
(providers, config). This is Kubby's differentiator — and the only feature that
sends cluster data off the machine, so read the data-handling section.

## Shape

An **Ask AI** tab in the resource drawer. A conversation about *that* resource:
Kubby collects its events, recent logs and manifest, attaches them to every
question, and shows the user exactly what it collected.

```go
DiagnosticContext(ctx, c, kind, ns, name) (*AIContext, error)
// AIContext{ Text, Events, LogContainers, LogLines, HasYAML, Chars }
```

The counts travel with the text so the header can state
`4 events · 120 log lines from 2 containers · manifest YAML`, next to a
**"See exactly what is sent"** viewer that prints the verbatim blob. An AI answer
is only worth trusting if you can see what it was based on — that is the design
principle here, not a nicety.

Evidence is capped: `diagLogTail` 60 lines, `diagLogChars` 3000, `diagYAMLChars`
6000. Logs are tail-truncated, YAML head-truncated.

## Conversation model

`AskAboutResource(kind, ns, name, history)` puts the **evidence in the system
prompt** and the conversation in `messages`.

That split matters: every follow-up is grounded in the same snapshot the thread
started with, while the frontend only has to replay the visible thread. Nothing is
persisted server-side; switching resources starts a fresh thread.

## Providers

`callAI` dispatches on `cfg.Provider`; all three take the same `[]AIMessage`.

| Provider | Endpoint | Default model |
|---|---|---|
| `anthropic` | `/v1/messages` | `claude-haiku-4-5-20251001` |
| `openai` | `/chat/completions` off a resolved base | `gpt-4o-mini` |
| `ollama` | `/api/chat` | `llama3.1` |

**`openAIBaseURL` exists because compatible endpoints disagree on the version
segment.** OpenAI and Groq end in `/v1`, Gemini's compatibility layer ends in
`/v1beta/openai`, internal gateways mount anywhere. Only a bare host gets `/v1`
appended; a URL that already carries a path is used verbatim. Appending `/v1`
blindly produced a 404 on Gemini — that was a real bug.

Ollama specifics: a 10-minute timeout (CPU inference is slow, and there is no
per-token cost to protect against locally), and `num_ctx: 8192` because Ollama
defaults to 2048 and **silently drops the oldest tokens** — which here is the
system prompt carrying all the evidence, producing a confident answer from a
truncated snapshot.

## Data handling — read before changing

- Config, **including the API key**, lives at `%AppData%/kubby/ai.json`; its
  directory and file permissions are tightened to **0700/0600** where supported.
- `GetAIConfig()` and `GetAIStatus()` expose only settings metadata plus a
  `hasApiKey` boolean. The key is write-only from the WebView and leaving its
  field blank preserves the existing key for the same provider.
- Hosted endpoints that receive an API key must use HTTPS. Plain HTTP is
  accepted only for loopback development endpoints.
- The evidence goes only to the provider the user chose. Ollama is local: nothing
  leaves the machine.
- Unconfigured shows a **setup card, not an error**.

Before evidence reaches either the preview or a provider, `DiagnosticContext`
redacts every Secret `data`/`stringData` value and every literal `env[].value`
while preserving field names and `valueFrom` references. It also filters common
credential shapes from events and logs (Bearer tokens and password/token/API-key
assignments). Tests use unique sentinels to prevent those values from returning.

This is defense in depth, not a proof that arbitrary free-form logs can never
contain sensitive application data. The exact post-redaction payload remains
visible through **See exactly what is sent** and should be reviewed before using
a hosted provider on a sensitive cluster.

## Verify

```powershell
go run ./cmd/kubby-cli diag Pod <name> -n <ns>
go run ./cmd/kubby-cli diag Gateway.networking.istio.io <name> -n <ns>
```

Prints the exact blob the tab would send, plus its stats line — no AI call, no key
needed. This is the right way to check evidence collection after a change.
