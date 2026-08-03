# Multi-source news digest

## Decisions

- The digest remains one aggregated email.
- Hacker News and IEEE Spectrum are built-in and active by default.
- `TOP_STORIES` is a global cap, merged with deterministic round-robin ordering.
- A failed provider is isolated; the run continues when another provider returns usable stories.
- The run fails only when every provider fails or returns no usable stories.
- IEEE Spectrum uses its latest official RSS feed and accepts only article links on `spectrum.ieee.org`.

## Normalized contract

Providers return `news.Story` values with source-qualified string IDs, source metadata, article URL, provider permalink, fallback text, and optional engagement metrics. The LLM and email layers consume this normalized contract. Legacy numeric Hacker News IDs from the model are normalized when unambiguous.

## Safety and observability

RSS input is bounded, article URLs are validated, and source errors never expose URLs or credentials in logs. Provider start/success/failure, aggregation counts, and downstream source-qualified story IDs are logged as structured events.

## Extension point

Adding a source requires implementing `news.Provider`, adding focused parser and failure tests, and registering the provider in `cmd/daily-digest-news`. No arbitrary source URL is accepted through environment variables.
