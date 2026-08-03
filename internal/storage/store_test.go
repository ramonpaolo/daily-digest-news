package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
)

func TestStorePersistsLessonAndDeduplicatesScheduledSlotAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lessons.sqlite3")
	now := time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC)
	lesson := llm.Lesson{
		Title: "Filas", Topic: "matemática", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
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
	if len(history) != 1 || history[0].Title != "Filas" {
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
		Title: "Velocidade", Topic: "física", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
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
