# Implementation Plan for Lucipedia

## 1. Baseline & Configuration
- Keep the existing Go module layout (`cmd/server`, `internal/...`) and ensure every package exposes clear, typed APIs.
- Extend `internal/config` so all runtime values come from environment variables (DB path, server port, log level, LLM endpoint/key, model list, Sentry DSN, search limits).
- Keep configuration parsing verbose and validated with `eris` errors that explain which variable failed and why.
- Document required variables in `.env.example` and README once the wiring is complete.

## 2. Logging & Observability
- Finalise the `internal/log` package by wiring logrus JSON logging, structured field helpers, and Sentry integration gated by configuration.
- Ensure every place that returns an error logs it with contextual fields (slug, query, request id, etc.) before bubbling it up.
- Add request-scoped logging helpers in the HTTP layer so handlers consistently attach context.

## 3. Database & Persistence Layer
- Use `internal/db` to open SQLite with WAL mode enabled and expose a single `pages` table.
- Expand the `wiki.Page` model with fields for the HTML text and timestamps.
- Add a startup migration (`AutoMigrate`) and make sure schema changes are logged and wrapped with `eris` on failure.
- Broaden the repository to support fetching by slug, upserting a page, and listing pages for housekeeping tasks.
- Provide lightweight repository tests using an in-memory SQLite database to guard against regressions.

## 4. LLM Content Generation
- Define a `Generator` interface in `internal/llm` with `Generate(ctx, slug) (HTML string, backlinks []string, err error)`.
- Implement `openrouter.Client` wrapping `openai-go v2.7.0`, configured via environment variables (base URL, API key, default model) and instrumented with logrus + eris.
- Build a prompt composer that receives the slug, optional context backlinks, and returns deterministic prompts used by the OpenRouter chat completion API.
- Decode successful responses as raw HTML, extract backlinks by scanning `/wiki/{slug}` anchors, and continue handling safety filter/tool call responses explicitly.
- Provide a simple mock generator for unit tests and deterministic fixtures for HTML/backlink content.

## 5. Search Experience
- Start with a simple text search that scans stored pages and highlights matching slugs or headings.
- Add configuration flags controlling search result limits and throttling behaviour.
- Instrument search operations with timing logs so accuracy and latency can be iterated on later.

## 6. Domain Service
- Implement `internal/wiki.Service` with dependencies on the repository, generator, and logger/Sentry hub.
- Expose `GetPage(ctx, slug)` to fetch or lazily generate pages: check cache/database, call the generator on misses, validate backlinks (ensure `/wiki/...` links), save the page, and return HTML.
- Expose `Search(ctx, query, limit)` to query stored pages using the simple matching logic introduced in the search experience.
- Make sure every branch wraps and logs errors with the relevant slug or query for traceability.

## 7. HTTP Transport Layer
- Build handlers in `internal/http` for:
  - `GET /wiki/{slug}` returning the stored HTML with `Content-Type: text/html`.
  - `GET /search?q=` rendering an HTML results page that links to `/wiki/{slug}` entries ranked by similarity.
  - `GET /healthz` exposing a simple status check (DB ping, generator readiness).
- Add middleware for request logging, panic recovery, request IDs, and Sentry tracing.
- Ensure handlers translate domain errors into meaningful HTTP responses (404 when a page truly does not exist, 500 for unexpected errors).

## 8. Application Wiring & Lifecycle
- Complete `cmd/server/main.go` to load config, initialise logging/Sentry, open the database, run migrations, build repository + services, and start the HTTP server.
- Use `context` cancellation and `http.Server.Shutdown` for graceful shutdown, flushing Sentry and closing the DB connection on exit.
- Surface startup/shutdown failures with `eris` wrapping and log them before exiting.

## 9. Testing Strategy
- Add table-driven unit tests for configuration parsing, DB repository operations, LLM generator fallback logic, domain service flows, and HTTP handlers.
- Create an integration test that boots the service with an in-memory DB and mock LLM components to cover the full `/wiki/{slug}` and `/search` flows.
- Include fixtures for generated HTML with backlinks to make assertions straightforward.
- Offer an opt-in live generator test to manually validate OpenRouter performance end-to-end.

## 10. Developer Experience & Tooling
- Provide `Makefile` targets for `fmt`, `lint`, `test`, and `run` to keep workflows simple.
- Integrate `golangci-lint` with rules that encourage verbose, straightforward code.
- Update the README with setup instructions, environment variable descriptions, and examples of wiki/search usage once the endpoints are live.

## 11. Containerization
- Create a multi-stage Dockerfile that compiles the server binary, runs unit tests during the build, and produces a minimal runtime image bundling configuration defaults.
- Generate a `docker-compose.yml` that mounts the SQLite data directory, loads environment variables from `.env`, and exposes the HTTP port.
- Document container workflows (build, run, tear down) in the README and keep them aligned with the Makefile targets.
