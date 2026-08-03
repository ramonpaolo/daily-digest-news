# System Design and Engineering Blogs design

## Summary

Add two independent email flows to the existing private Go job:

- a daily System Design lesson at 06:30 in the configured timezone;
- a weekly engineering-blog digest on Sunday at 20:00.

Both flows reuse the existing SMTP, Zenifra AI, HTTP, fetcher, scheduler, `/data` volume, and structured logging conventions. They remain isolated from news and the existing learning runner: one failure does not suppress another email.

## Content contracts

System Design uses a dedicated LLM contract. The model receives the current slot and recent subjects, then returns a Portuguese lesson with a concrete subject, opening, explanation, mechanics, trade-offs, failure modes, example, and 3–5 takeaways. Prompts explicitly reject interview coaching, management, and superficial recipes. Initial domains rotate through distribution, replication, partitioning, consistency, consensus, queues/streaming, cache, storage/indexes, networking/backpressure, idempotency/retries, and observability/SLOs.

Engineering blogs use a two-stage LLM flow. First, candidate metadata is ranked and up to three new technical/research articles are selected. Then only those articles are extracted and summarized in depth. The resulting email includes source, title, summary, why it matters, key technical ideas, trade-offs, and the original link.

## Sources and editorial policy

`ENGINEERING_BLOG_SOURCES` replaces defaults using `Name|URL` entries separated by semicolons. Defaults are Uber Engineering (`https://eng.uber.com/`, with an official-page fallback when RSS is unavailable), Netflix Technology Blog (`https://netflixtechblog.com/feed`), Discord (`https://discord.com/blog/rss.xml`), Cloudflare (`https://blog.cloudflare.com/rss/`), and Slack Engineering (`https://slack.engineering/feed/`).

RSS/Atom candidates are filtered to engineering and research topics; hiring, marketing, product, and policy content is excluded. Uber's HTML fallback accepts only `BLOG`, `RESEARCH`, and `OSS` entries. The weekly window covers the preceding seven days. Up to three unseen articles are selected; one or two are acceptable when fewer exist. With no new usable article, the run is recorded as skipped and no empty email is sent.

## Persistence and idempotency

SQLite receives migrations for System Design lessons and engineering-blog runs/items. System Design deduplicates by concrete subject. Blog items deduplicate by source plus GUID or canonical URL. Existing records remain readable, and the existing `LEARNING_RETENTION_DAYS` cleanup policy applies to both new histories.

## Failure and safety behavior

Each runner claims its daily/weekly slot before external work, stores generated content before SMTP, and marks records `sent` or `failed`. Existing three-attempt retry/backoff behavior is reused. Feed failures are isolated per source. Article text is untrusted input: prompts delimit it and prohibit following instructions found inside articles. Logs expose only source names, counts, phases, durations, and safe identifiers; never prompts, article bodies, tokens, or email bodies.
