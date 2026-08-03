# System Design and Engineering Blogs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add isolated daily System Design and weekly engineering-blog emails with LLM summaries, source filtering, SQLite history, and idempotent scheduling.

**Architecture:** Add dedicated LLM contracts, email renderers, storage methods, and runners. Reuse the existing HTTP client, fetcher, SMTP mailer, scheduler, retry policy, and `/data` SQLite volume without changing the news or existing learning flow.

**Tech Stack:** Go 1.26.5, `net/http`, `encoding/xml`, existing readability fetcher, Zenifra OpenAI-compatible API, SQLite via `modernc.org/sqlite`, SMTP.

## Global Constraints

- Keep project exposure private and persistence under `/data`.
- Use the existing five-minute AI timeout and three-attempt retry/backoff policy.
- Never log prompts, article text, email bodies, credentials, tokens, or local email parts.
- Preserve existing news and learning behavior and apply `LEARNING_RETENTION_DAYS` to new history.
- Do not commit, push, or deploy without explicit user authorization.

### Task 1: Add failing LLM and email contract tests

**Files:**
- Modify: `internal/llm/lesson_test.go`, `internal/email/lesson_test.go`
- Create: `internal/llm/system_design_test.go`, `internal/llm/engineering_blog_test.go`, `internal/email/system_design_test.go`, `internal/email/engineering_blog_test.go`

- [ ] Add tests for System Design JSON validation, required technical sections, subject dedup metadata, and prompt safety policy.
- [ ] Add tests for blog candidate selection returning at most three exact IDs and blog summary validation preserving IDs.
- [ ] Add tests for readable text/HTML renderers and dedicated subject lines.
- [ ] Run focused tests and confirm RED failures for missing types/functions.

### Task 2: Implement LLM contracts and prompts

**Files:**
- Modify: `internal/llm/client.go`
- Create: `internal/llm/system_design.go`, `internal/llm/engineering_blogs.go`

- [ ] Define `SystemDesignRequest`, `SystemDesignHistory`, and `SystemDesignLesson` with exact fields `subject`, `opening`, `content`, `mechanics`, `tradeoffs`, `failure_modes`, `example`, and `takeaways`.
- [ ] Implement `GenerateSystemDesign` using the five-minute client, Portuguese conceptual prompt, recent-history metadata, and local validation.
- [ ] Define blog candidate and digest contracts with exact source IDs and validate selected IDs, 1–3 items, nonempty summaries, technical ideas, and links.
- [ ] Implement metadata selection and full-article summarization prompts with untrusted-content boundaries and no administrative/business instructions.
- [ ] Run `go test ./internal/llm ./internal/email` and confirm GREEN.

### Task 3: Implement RSS/Atom and Uber HTML source adapters

**Files:**
- Create: `internal/news/engineering_blogs.go`
- Create: `internal/news/engineering_blogs_test.go`
- Modify: `internal/fetch/article.go` only if a bounded helper is required; preserve SSRF protections.

- [ ] Parse RSS 2.0 and Atom title, link, GUID/ID, description/summary, categories, and publication time with a 2 MiB response limit.
- [ ] Normalize source IDs as `source:guid` or `source:canonical-url`, reject unsafe URLs, and filter candidates to the preceding seven-day window.
- [ ] Implement Uber HTML fallback for official page entries, accepting `BLOG`, `RESEARCH`, and `OSS` labels and rejecting `NEWS`/`HIRING`.
- [ ] Return partial results when one source fails and a clear all-source error only when no source yields candidates.
- [ ] Test malformed feeds, duplicate URLs, category filtering, date windows, Uber fallback, and source isolation.

### Task 4: Add email renderers

**Files:**
- Create: `internal/email/system_design.go`, `internal/email/engineering_blogs.go`
- Create: corresponding renderer tests

- [ ] Render System Design with subject, explanation, mechanics, trade-offs, failure modes, example, and takeaways in text and escaped HTML.
- [ ] Render the weekly blog digest with source/title links, summaries, technical ideas, trade-offs, and why-it-matters sections.
- [ ] Use subjects `System Design — <subject> — <date>` and `Engineering Blogs — semana de <date>`.
- [ ] Reject incomplete content before SMTP.

### Task 5: Add SQLite migrations and storage APIs

**Files:**
- Modify: `internal/storage/store.go`, `internal/storage/store_test.go`

- [ ] Add migrations for `system_design_lessons`, `engineering_blog_runs`, and `engineering_blog_items` without modifying legacy lesson semantics.
- [ ] Implement cleanup, slot claims, recent System Design subjects, blog item deduplication, run status, and save/mark sent/failed methods.
- [ ] Backfill no new fields for legacy records; ensure migration re-open is idempotent.
- [ ] Test daily/weekly idempotency, source+URL deduplication, cleanup, and migration from schema version 2.

### Task 6: Add runners and scheduling

**Files:**
- Create: `internal/job/system_design.go`, `internal/job/engineering_blogs.go`
- Create: corresponding runner tests
- Modify: `cmd/daily-digest-news/main.go`

- [ ] Implement `SystemDesignRunner` with daily slot claim, history, generation, persistence, rendering, SMTP, and structured logs.
- [ ] Implement `EngineeringBlogRunner` with source collection, LLM candidate selection, bounded extraction, summary generation, persistence, rendering, SMTP, and partial-source handling.
- [ ] Add `SYSTEM_DESIGN_TIME=06:30` and `ENGINEERING_BLOG_WEEKLY_TIME=20:00` parsing; schedule the blog runner only on Sunday.
- [ ] Keep startup behavior unchanged: existing news and learning startup emails remain the only startup deliveries.
- [ ] Test runner isolation, retries, duplicate skips, no-new-article skips, and scheduler wiring.

### Task 7: Configuration, documentation, and full verification

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`, `README.md`
- Modify: `docs/superpowers/specs/2026-08-03-system-design-engineering-blogs-design.md`

- [ ] Add and validate the two schedule variables plus `ENGINEERING_BLOG_SOURCES` with the documented default sources.
- [ ] Document email schedules, source override syntax, `/data` persistence, editorial filters, and no-new-article behavior.
- [ ] Run `gofmt`, `go test -race ./...`, `go vet ./...`, `git diff --check`, and `docker build --pull=false --quiet -t daily-digest-news:local-system-design-blogs .`.
- [ ] Inspect the final diff and report implementation/validation status without publishing.
