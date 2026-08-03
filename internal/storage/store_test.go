package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	_ "modernc.org/sqlite"
)

func TestStoreMigratesLegacyLessonSubjectFromTitle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	legacySchema := `CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
INSERT INTO schema_migrations(version, applied_at) VALUES(1, '2026-08-02T00:00:00Z');
CREATE TABLE interests(id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE, active INTEGER NOT NULL DEFAULT 1, first_seen_at TEXT NOT NULL, last_used_at TEXT);
CREATE TABLE learning_lessons(id INTEGER PRIMARY KEY, date_key TEXT NOT NULL, slot TEXT NOT NULL, slot_key TEXT UNIQUE, topic TEXT NOT NULL, request_json TEXT NOT NULL, title TEXT, kind TEXT, opening TEXT, content TEXT, answer TEXT, takeaways_json TEXT, status TEXT NOT NULL, phase TEXT, error TEXT, attempts INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, generated_at TEXT, sent_at TEXT);
INSERT INTO learning_lessons(date_key, slot, topic, request_json, title, kind, opening, content, takeaways_json, status, created_at, updated_at, sent_at) VALUES('2026-08-02','morning','matemática','{}','Euler','text','Abertura','Conteúdo','[]','sent','2026-08-02T00:00:00Z','2026-08-02T00:00:00Z','2026-08-02T00:00:00Z');`
	if _, err := db.Exec(legacySchema); err != nil {
		db.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}
	store, err := Open(ctx, path, 365)
	if err != nil {
		t.Fatalf("Open() migration error = %v", err)
	}
	defer store.Close()
	history, err := store.RecentLessons(ctx, 1)
	if err != nil {
		t.Fatalf("RecentLessons() error = %v", err)
	}
	if len(history) != 1 || history[0].Subject != "Euler" {
		t.Fatalf("history = %+v, want legacy title backfilled as subject", history)
	}
}

func TestStorePersistsLessonAndDeduplicatesScheduledSlotAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lessons.sqlite3")
	now := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	lesson := llm.Lesson{
		Title: "Filas", Topic: "matemática", Subject: "Como funcionam filas", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
		Takeaways: []string{"um", "dois", "três"},
	}

	store, err := Open(ctx, path, 365)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.SyncInterests(ctx, []string{"matemática", "física"}, now); err != nil {
		t.Fatalf("SyncInterests() error = %v", err)
	}
	claim, err := store.BeginLesson(ctx, "2026-08-03", "morning", `{"topic":"matemática"}`, now)
	if err != nil {
		t.Fatalf("BeginLesson() error = %v", err)
	}
	if claim.AlreadySent || claim.ID == 0 || claim.Topic == "" {
		t.Fatalf("claim = %+v, want new lesson claim", claim)
	}
	if err := store.SaveGenerated(ctx, claim.ID, lesson, now); err != nil {
		t.Fatalf("SaveGenerated() error = %v", err)
	}
	if err := store.MarkSent(ctx, claim.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = Open(ctx, path, 365)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer store.Close()
	history, err := store.RecentLessons(ctx, 8)
	if err != nil {
		t.Fatalf("RecentLessons() error = %v", err)
	}
	if len(history) != 1 || history[0].Title != "Filas" || history[0].Subject != "Como funcionam filas" {
		t.Fatalf("history = %+v, want persisted lesson", history)
	}
	duplicate, err := store.BeginLesson(ctx, "2026-08-03", "morning", `{}`, now)
	if err != nil {
		t.Fatalf("duplicate BeginLesson() error = %v", err)
	}
	if !duplicate.AlreadySent {
		t.Fatalf("duplicate claim = %+v, want AlreadySent", duplicate)
	}
}

func TestStoreDoesNotDeduplicateStartupLessons(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "lessons.sqlite3"), 365)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	if err := store.SyncInterests(ctx, []string{"matemática"}, now); err != nil {
		t.Fatalf("SyncInterests() error = %v", err)
	}
	first, err := store.BeginLesson(ctx, "2026-08-03", "startup", `{}`, now)
	if err != nil {
		t.Fatalf("first BeginLesson() error = %v", err)
	}
	second, err := store.BeginLesson(ctx, "2026-08-03", "startup", `{}`, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second BeginLesson() error = %v", err)
	}
	if first.ID == second.ID || first.AlreadySent || second.AlreadySent {
		t.Fatalf("startup claims = %+v and %+v, want two new claims", first, second)
	}
}

func TestStoreCleanupRemovesExpiredLessonsAndKeepsInterests(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "lessons.sqlite3"), 30)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	if err := store.SyncInterests(ctx, []string{"física"}, old); err != nil {
		t.Fatalf("SyncInterests() error = %v", err)
	}
	claim, err := store.BeginLesson(ctx, "2026-01-01", "startup", `{}`, old)
	if err != nil {
		t.Fatalf("BeginLesson() error = %v", err)
	}
	if err := store.SaveGenerated(ctx, claim.ID, llm.Lesson{
		Title: "Velocidade", Topic: "física", Subject: "Como medimos velocidade", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
		Takeaways: []string{"um", "dois", "três"},
	}, old); err != nil {
		t.Fatalf("SaveGenerated() error = %v", err)
	}
	if err := store.MarkSent(ctx, claim.ID, old); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if err := store.Cleanup(ctx, now); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	history, err := store.RecentLessons(ctx, 8)
	if err != nil {
		t.Fatalf("RecentLessons() error = %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("history = %+v, want expired lesson removed", history)
	}
	if err := store.SyncInterests(ctx, []string{"física"}, now); err != nil {
		t.Fatalf("SyncInterests() after cleanup error = %v", err)
	}
}
