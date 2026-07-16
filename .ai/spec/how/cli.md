# `oc-ols` CLI — architecture (how)

Audience: AI agents. This document describes **code layout, client wiring, and I/O paths** for the OLS chat CLI plugin.

---

## Entry point: `cmd/oc-ols/main.go`

- Builds `genericclioptions.IOStreams` from `os.Stdin` / `Stdout` / `Stderr`.
- `cli.NewRootCmd(streams).Execute()` — Cobra root.

---

## Module map: `cli/`

| File | Types | Key functions |
|------|-------|---------------|
| `root.go` | — | `NewRootCmd(streams)` — registers `ask`, `troubleshoot` (default mode dispatching), `config` subtree, and `version` |
| `version.go` | Package var `Version` (default `dev`) | `NewVersionCmd(streams)` |
| `ask.go` | `AskOptions` | `NewAskCmd`, `Complete`, `Validate`, `Run` — streams query in `ask` mode |
| `troubleshoot.go` | `TroubleshootOptions` | `NewTroubleshootCmd`, `Complete`, `Validate`, `Run` — streams query in `troubleshooting` mode |
| `streaming.go` | `SSEClient` | `NewSSEClient`, `StreamQuery` — shared HTTP + SSE streaming logic |
| `attachments.go` | — | `ReadAttachments(paths)` — reads files, builds attachment array |
| `render.go` | — | `RenderMarkdown(text)` — terminal markdown rendering via glamour |

*File names are planned. Update during implementation.*

---

## Module map: `cli/config/`

| File | Types | Key functions |
|------|-------|---------------|
| `endpoint.go` | `SetEndpointOptions` | `NewSetEndpointCmd`, `Run` — persists URL keyed by kubeconfig context |
| `persistence.go` | `ContextStore` | `LoadConversationID`, `SaveConversationID`, `LoadEndpoint`, `SaveEndpoint` — local file storage per kubeconfig context |

---

## Command tree

```
oc ols "question"                          # default ask mode, streaming
oc ols ask "question"                      # explicit ask mode
oc ols troubleshoot "question"             # troubleshoot mode
oc ols config set-endpoint <URL>           # set endpoint for current kubeconfig context
oc ols version                             # print version

Global flags:
  --endpoint <URL>                         # override endpoint for this invocation
  --file <path>                            # attach file(s) — StringSlice
  --conversation-id <UUID>                 # continue specific conversation
  --new                                    # start fresh conversation
  --output json                            # structured JSON output
  --insecure-skip-tls-verify               # skip TLS verification
  --ca-cert <path>                         # custom CA certificate
  --kubeconfig <path>                      # kubeconfig file (standard)
```

Default mode dispatching: when the first positional argument does not match a registered subcommand, the root command treats it as a query string and dispatches to `ask` mode.

---

## Service client & streaming

**No Kubernetes API interaction beyond kubeconfig.** Unlike `oc-agentic` (which uses controller-runtime typed client, dynamic client, and clientset for CRD CRUD), `oc-ols` is a pure REST API client:

- **`k8s.io/client-go/tools/clientcmd`**: Read kubeconfig, extract bearer token, extract TLS configuration (CA bundle, skip-verify). No `controller-runtime`, no CRD types, no API server calls.
- **`net/http`**: Build HTTP client with TLS config from kubeconfig. POST to lightspeed-service `/v1/streaming_query`.
- **SSE streaming**: Read `text/event-stream` response via buffered reader. Parse SSE event frames (`event:`, `data:` lines) into typed events.

---

## Kubeconfig integration

- `clientcmd.NewNonInteractiveDeferredLoadingClientConfig` for kubeconfig loading.
- Bearer token extracted from the resolved kubeconfig context (equivalent to `oc whoami -t`).
- TLS settings inherited from kubeconfig context: CA certificate, insecure-skip-tls-verify.
- Override flags: `--insecure-skip-tls-verify` and `--ca-cert <path>` take precedence over kubeconfig values.
- `--kubeconfig` flag for non-default kubeconfig file path.

---

## Endpoint configuration & service discovery

The oc-ols CLI does **not** auto-discover the lightspeed-service endpoint. This is the most significant architectural departure from `oc-agentic`, which communicates with the K8s API server directly.

**Service exposure model:**

1. An administrator creates an OpenShift Route for `lightspeed-app-server` in the `openshift-lightspeed` namespace (per existing OLS documentation — not automated by the operator).
2. The user configures the CLI with the Route URL: `oc ols config set-endpoint https://lightspeed.apps.cluster.example.com`.
3. The endpoint is stored per kubeconfig context in local file storage.

**Resolution order:**

1. `--endpoint <URL>` flag (highest priority, per-invocation override)
2. Persisted endpoint for current kubeconfig context
3. Error with guidance (see below)

**First-run behavior:** When no endpoint is configured and no `--endpoint` flag is provided, the CLI exits with:

```
Error: No endpoint configured for context "<context-name>".
Run: oc ols config set-endpoint <URL>
```

The CLI does not attempt to guess or discover the endpoint.

---

## Per-command API behavior

- **`ask` / default mode:** Builds `LLMRequest` with `mode: "ask"`, `query`, optional `conversation_id` (persisted), optional `attachments`. POST to `/v1/streaming_query` with `media_type: "application/json"`. Streams tokens to stdout via markdown renderer. On `end` event: display referenced documents. Persist returned `conversation_id`.
- **`troubleshoot`:** Same as `ask` but with `mode: "troubleshooting"`.
- **`config set-endpoint`:** Validates URL format. Writes to local storage keyed by current kubeconfig context name. Prints confirmation.
- **`version`:** Prints `Version` package variable (injected via ldflags at build time).

---

## Output formatting

- **Default (streaming text):** Tokens streamed to stdout through a terminal markdown renderer (see below). After the answer completes, referenced documents (`doc_url`, `doc_title`) displayed below the response.
- **`--output json`:** Full structured `LLMResponse` printed as indented JSON: `conversation_id`, `response`, `referenced_documents`, `truncated`, `input_tokens`, `output_tokens`, `available_quotas`. Bypasses markdown rendering.

---

## Terminal output rendering

LLM responses contain markdown formatting (headings, code blocks, lists, bold/italic). The CLI renders this for terminal readability.

- **Library:** [glamour](https://github.com/charmbracelet/glamour) (charmbracelet) — recommended. Auto-detects terminal width and color support.
- **Default behavior:** Rendered markdown output with ANSI styling (headings, code highlighting, list formatting).
- **`--output json`:** Bypasses rendering entirely — returns raw structured response.
- **Non-TTY detection:** When stdout is piped to a file or another process (not a terminal), glamour auto-falls back to plain text without ANSI codes.
- **Streaming consideration:** Tokens arrive incrementally via SSE. Buffer the full response text before rendering, or use glamour's word-wrap on the final output. Individual token streaming to stdout uses raw text; the rendered version is applied to the complete response.

---

## Conversation persistence

- **Storage location:** `~/.config/oc-ols/contexts/<context-name>/` directory.
  - `conversation.json` stores `{"conversation_id": "<uuid>", "updated_at": "<timestamp>"}`.
  - `endpoint` file stores the configured URL as plain text.
- **Behavior:** After each successful query, the returned `conversation_id` is persisted for the current kubeconfig context. On subsequent queries, the persisted `conversation_id` is included in the request automatically.
- **User notification:** The CLI always prints `"Continuing conversation <id>..."` to stderr when using a persisted conversation ID. This ensures the user knows they are in a multi-turn conversation.
- **`--new` flag:** Ignores persisted `conversation_id`, starts a fresh conversation. The new `conversation_id` from the response replaces the persisted one.
- **`--conversation-id <UUID>` flag:** Overrides persisted value for this invocation. The provided ID is used in the request; the response ID replaces the persisted one.
- **Cleanup:** No automatic cleanup. Users can delete `~/.config/oc-ols/contexts/<context-name>/conversation.json` to reset.

---

## File attachments

- `--file` flag accepts `StringSlice`: `--file a.yaml --file b.log` or `--file a.yaml,b.log`.
- Each file is read from disk, content included in `LLMRequest.attachments[]`.
- **Type inference:** `.yaml` / `.yml` / `.json` → `attachment_type: "configuration"`; all other extensions → `attachment_type: "log"`.
- **Content type:** `content_type: "text/plain"` for all attachments.

---

## SSE event handling

The lightspeed-service `/v1/streaming_query` endpoint returns Server-Sent Events. The CLI maps each event type to output behavior:

| SSE Event | CLI Output Behavior |
|-----------|-------------------|
| `start` | No visible output. Internal: note stream has begun, capture `conversation_id`. |
| `token` | Print `data` field content to stdout immediately (raw streaming). |
| `reasoning` | No visible output in default mode. Captured for `--output json`. |
| `tool_call` | No visible output in default mode. Captured for `--output json`. |
| `end` | Stop streaming. Parse `data` for `conversation_id` (persist), `referenced_documents` (display), and full response metadata (available via `--output json`). |

In default text mode, the user sees only the streamed tokens followed by referenced documents. The `reasoning` and `tool_call` events are implementation details of the service's internal processing. Full event data is available via `--output json`.

---

## Data flow

```
User invokes: oc ols "why is my pod crashing" --file pod.yaml
  │
  ├─ Cobra dispatches to ask mode Run()
  │    ├─ Complete():
  │    │    ├─ Load kubeconfig → extract bearer token + TLS config
  │    │    ├─ Resolve endpoint (flag > persisted > error)
  │    │    ├─ Load persisted conversation_id for current context
  │    │    └─ Read file attachments from --file paths
  │    ├─ Validate(): check endpoint resolved, query non-empty
  │    └─ Run():
  │         ├─ Print "Continuing conversation <id>..." to stderr (if continuing)
  │         ├─ Build LLMRequest:
  │         │    query: "why is my pod crashing"
  │         │    mode: "ask"
  │         │    conversation_id: "<persisted-uuid>"
  │         │    attachments: [{content of pod.yaml}]
  │         │    media_type: "application/json"
  │         ├─ POST /v1/streaming_query with Authorization: Bearer <token>
  │         ├─ Read SSE stream:
  │         │    token events → print to stdout (raw text)
  │         │    end event → extract conversation_id, referenced_documents
  │         ├─ Render complete response through glamour (if TTY)
  │         ├─ Display referenced documents
  │         └─ Persist new conversation_id for context
  │
  └─ Output: rendered markdown + references (default) or full LLMResponse (--output json)
```

---

## Key abstractions

- **`ContextStore`** — Local file storage keyed by kubeconfig context name. Manages endpoint URLs and conversation IDs. No K8s API interaction.
- **`SSEClient`** — HTTP client with bearer auth and TLS config. Sends POST requests to lightspeed-service, reads SSE event streams, dispatches events to handler callbacks.
- **Attachment reader** — Reads files from disk, infers `attachment_type` from extension, builds `LLMRequest.attachments[]`.
- **Markdown renderer** — glamour-based terminal renderer. Auto-detects TTY, falls back to plain text when piped.
- **No typed K8s client** — Unlike `oc-agentic`, there is no `controller-runtime` client, no CRD types, no scheme registration. The only K8s dependency is `client-go` for kubeconfig parsing.

---

## Differences from agentic CLI

| Aspect | oc-agentic | oc-ols |
|--------|-----------|--------|
| K8s client | controller-runtime (typed CRD client) + dynamic + clientset | client-go only (kubeconfig/token/TLS) |
| API target | K8s API server (AgenticRun CRDs) | lightspeed-service REST API (HTTP) |
| Command depth | `run {create,list,get,...}` + system commands | `{ask,troubleshoot}` + config + version |
| State | Stateless (reads CRDs) | Conversation persistence (local file storage) |
| Service discovery | N/A (talks to K8s API) | Admin-created Route, user-configured endpoint |
| Output | Tables, colored phases, JSON/YAML | Rendered markdown + references, JSON |

---

## Error handling

| Error Class | User-Facing Behavior |
|-------------|---------------------|
| No endpoint configured | `Error: No endpoint configured for context "<name>". Run: oc ols config set-endpoint <URL>` — exit code 1 |
| Authentication failure (401) | `Error: Authentication failed. Is your login session active? Try: oc login` — exit code 1 |
| Authorization denied (403) | `Error: Access denied. Contact your cluster administrator to grant OLS access.` — exit code 1 |
| Network / TLS error | `Error: Could not connect to <endpoint>: <detail>` — exit code 1 |
| Service error (non-200) | `Error: Service returned <status>: <detail>` — exit code 1 |
| SSE stream interrupted | Partial output printed to stdout + `Warning: Response may be incomplete (stream interrupted)` to stderr — exit code 1 |
| Prompt too long (413) | `Error: Query exceeds maximum length. Try a shorter question or fewer attachments.` — exit code 1 |

---

## Cross-references

- CLI binary distribution: **how/cli-distribution.md**
- Lightspeed service REST API (cross-repo): **lightspeed-service** `what/api.md` — request schema, SSE event types, response fields
- Lightspeed service auth (cross-repo): **lightspeed-service** `what/auth.md` — TokenReview + SubjectAccessReview against `/ols-access`

---

## Implementation notes

*Placeholder for findings during implementation. Update with actual file names, type signatures, and discovered constraints as implementation proceeds under OLS-1062.*
