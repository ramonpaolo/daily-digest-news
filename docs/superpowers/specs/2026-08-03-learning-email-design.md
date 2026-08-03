# Learning email design

## Decisions

- The learning flow sends one email at startup and then at 07:00 and 18:00 in the configured timezone.
- News and learning are independent runners; a learning failure never blocks news.
- Topics come from `LEARNING_TOPICS`, with built-in defaults for mathematics, computing, Go, data structures and algorithms, operating-system internals, database internals, and physics.
- Learning history is persisted in SQLite; active interests are selected by least recent use, while the news digest remains transient.
- Lessons are 10–15 minute, self-contained Portuguese-Brazilian content. The model chooses a question or explanatory text; questions always include a commented answer.

## Contracts

`llm.LessonRequest` carries the configured topic and slot. `llm.Lesson` contains `title`, `topic`, `kind`, `opening`, `content`, conditional `answer`, and 3–5 `takeaways`. The client rejects mismatched topics, unsupported kinds, incomplete answers, oversized fields, and invalid takeaway counts.

`job.LearningRunner` owns startup/slot guards, topic rotation, retries, rendering, SMTP delivery, and content-free structured logs. `email.RenderLesson` produces a separate subject and text/HTML message without changing the news renderer.

## Scheduling and rollout

The existing news scheduler remains at `SCHEDULE_TIME` (08:00 by default). Two additional scheduler adapters invoke `RunSlot` for `morning` and `evening`; startup invokes `RunStartup` once. SQLite uses the persistent `/data` volume and does not require new secrets. The image pipeline and Zenifra rollout remain the same after tests pass.
