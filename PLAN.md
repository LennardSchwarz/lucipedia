# Implementation Plan for Lucipedia

## 1. Baseline & Configuration
- Keep the existing Go module layout (`cmd/server`, `internal/...`) and ensure every package exposes clear, typed APIs.
- Extend `internal/config` so all runtime values come from environment variables (DB path, server port, log level, LLM endpoint/key, model list, Sentry DSN, embedding model, search limits).
- Keep configuration parsing verbose and validated with `eris` errors that explain which variable failed and why.
- Document required variables in `.env.example` and README once the wiring is complete.

## 2. Logging & Observability
- Finalise the `internal/log` package by wiring logrus JSON logging, structured field helpers, and Sentry integration gated by configuration.
- Ensure every place that returns an error logs it with contextual fields (slug, query, request id, etc.) before bubbling it up.
- Add request-scoped logging helpers in the HTTP layer so handlers consistently attach context.

## 3. Database & Persistence Layer
- Use `internal/db` to open SQLite with WAL mode enabled and expose a single `pages` table.
- Expand the `wiki.Page` model with fields for the HTML text, optional raw embedding vector (e.g. `[]byte` storing JSON float array), and timestamps.
- Add a startup migration (`AutoMigrate`) and make sure schema changes are logged and wrapped with `eris` on failure.
- Broaden the repository to support: fetching by slug, upserting a page, listing pages, and retrieving embeddings for search.
- Provide lightweight repository tests using an in-memory SQLite database to guard against regressions.

## 4. LLM Content Generation
- Define a `Generator` interface in `internal/llm` with `Generate(ctx, slug) (HTML string, backlinks []string, err error)`.
- Implement `openrouter.Client` wrapping `openai-go v2.7.0`, configured via environment variables (base URL, API key, default model) and instrumented with logrus + eris.
- Build a prompt composer that receives the slug, optional context backlinks, and returns deterministic prompts used by the OpenRouter chat completion API.
- Decode successful responses into structured HTML/backlink payload, handling safety filter/tool call responses explicitly.
- Provide a simple mock generator for unit tests and deterministic fixtures for HTML/backlink content.

## 5. Embeddings & Search
- Introduce an `Embedder` interface that can embed both full page HTML and free-form queries; implement it using the OpenRouter embeddings endpoint via the same client as the generator.
- Add an `EmbeddingsClient` in `internal/llm` with methods `EmbedPage(ctx, slug, html)` and `EmbedQuery(ctx, query)` returning `[]float32`, reusing shared configuration and logging.
- Extend repository usage to persist embeddings returned by the embedder and expose them for search without re-calling the LLM.
- Build a search component (inside `internal/wiki` or a dedicated `internal/search` package) that loads embeddings, computes cosine similarity, and returns the top-K slugs.
- Cache embeddings in memory on startup for fast KNN queries and refresh the cache whenever a page is regenerated.
- Add basic metrics/logging for search latency and hit counts to aid tuning.

## 6. Domain Service
- Implement `internal/wiki.Service` with dependencies on the repository, generator, embedder, and logger/Sentry hub.
- Expose `GetPage(ctx, slug)` to fetch or lazily generate pages: check cache/database, call the generator on misses, validate backlinks (ensure `/wiki/...` links), save the page, compute embeddings, and return HTML.
- Expose `Search(ctx, query, limit)` to embed the query, run KNN against stored embeddings, hydrate page summaries, and return ordered results.
- Make sure every branch wraps and logs errors with the relevant slug or query for traceability.

## 7. HTTP Transport Layer
- Build handlers in `internal/http` for:
  - `GET /wiki/{slug}` returning the stored HTML with `Content-Type: text/html`.
  - `GET /search?q=` rendering an HTML results page that links to `/wiki/{slug}` entries ranked by similarity.
  - `GET /healthz` exposing a simple status check (DB ping, optional embedder/generator readiness).
- Add middleware for request logging, panic recovery, request IDs, and Sentry tracing.
- Ensure handlers translate domain errors into meaningful HTTP responses (404 when a page truly does not exist, 500 for unexpected errors).

## 8. Application Wiring & Lifecycle
- Complete `cmd/server/main.go` to load config, initialise logging/Sentry, open the database, run migrations, build repository + services, and start the HTTP server.
- Use `context` cancellation and `http.Server.Shutdown` for graceful shutdown, flushing Sentry and closing the DB connection on exit.
- Surface startup/shutdown failures with `eris` wrapping and log them before exiting.

## 9. Testing Strategy
- Add table-driven unit tests for configuration parsing, DB repository operations, LLM generator fallback logic, embedding calculations, domain service flows, and HTTP handlers.
- Create an integration test that boots the service with an in-memory DB and mock LLM components to cover the full `/wiki/{slug}` and `/search` flows.
- Include fixtures for generated HTML with backlinks and deterministic embedding vectors to make assertions straightforward.

## 10. Developer Experience & Tooling
- Provide `Makefile` targets for `fmt`, `lint`, `test`, and `run` to keep workflows simple.
- Integrate `golangci-lint` with rules that encourage verbose, straightforward code.
- Update the README with setup instructions, environment variable descriptions, and examples of wiki/search usage once the endpoints are live.

## 11. Containerization
- Create a multi-stage Dockerfile that compiles the server binary, runs unit tests during the build, and produces a minimal runtime image bundling configuration defaults.
- Generate a `docker-compose.yml` that mounts the SQLite data directory, loads environment variables from `.env`, and exposes the HTTP port.
- Document container workflows (build, run, tear down) in the README and keep them aligned with the Makefile targets.
