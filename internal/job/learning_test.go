package job

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/storage"
)

type fakeLessonGenerator struct {
	requests []llm.LessonRequest
	lesson   llm.Lesson
	err      error
}

func (f *fakeLessonGenerator) GenerateLesson(_ context.Context, request llm.LessonRequest) (llm.Lesson, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return llm.Lesson{}, f.err
	}
	return f.lesson, nil
}

func TestLearningRunnerSendsStartupAndScheduledLessonsIndependently(t *testing.T) {
	generator := &fakeLessonGenerator{lesson: llm.Lesson{
		Title: "Título", Topic: "matemática", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
		Takeaways: []string{"um", "dois", "três"},
	}}
	mailer := &fakeMailer{}
	store := openLearningStore(t)
	runner := NewLearningRunner(generator, mailer, store, []string{"matemática", "física"})
	runner.now = func() time.Time { return time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC) }
	runner.sleep = func(context.Context, time.Duration) error { return nil }

	if err := runner.RunStartup(context.Background()); err != nil {
		t.Fatalf("RunStartup() error = %v", err)
	}
	if err := runner.RunSlot(context.Background(), "morning"); err != nil {
		t.Fatalf("RunSlot() error = %v", err)
	}
	if err := runner.RunSlot(context.Background(), "morning"); err != nil {
		t.Fatalf("second morning RunSlot() error = %v, want idempotent skip", err)
	}
	if len(mailer.sent) != 2 || len(generator.requests) != 2 {
		t.Fatalf("sent=%d requests=%d, want startup plus scheduled lesson", len(mailer.sent), len(generator.requests))
	}
	if generator.requests[0].Slot != "startup" || generator.requests[1].Slot != "morning" {
		t.Fatalf("requests = %+v", generator.requests)
	}
	if len(generator.requests[1].RecentLessons) != 1 || generator.requests[1].RecentLessons[0].Title != "Título" {
		t.Fatalf("morning history = %+v, want startup lesson metadata", generator.requests[1].RecentLessons)
	}
}

func TestLearningRunnerFailureDoesNotMarkSlotSuccessful(t *testing.T) {
	generator := &fakeLessonGenerator{err: errors.New("AI unavailable")}
	runner := NewLearningRunner(generator, &fakeMailer{}, openLearningStore(t), []string{"física"})
	runner.now = func() time.Time { return time.Date(2026, 8, 3, 18, 0, 0, 0, time.UTC) }
	runner.sleep = func(context.Context, time.Duration) error { return nil }

	if err := runner.RunSlot(context.Background(), "evening"); err == nil {
		t.Fatal("RunSlot() succeeded, want generator error")
	}
	if err := runner.RunSlot(context.Background(), "evening"); err == nil {
		t.Fatal("second RunSlot() succeeded, want retryable failed slot")
	}
}

func TestLearningRunnerLogsNoLessonContent(t *testing.T) {
	var logs strings.Builder
	generator := &fakeLessonGenerator{lesson: llm.Lesson{
		Title: "Segredo", Topic: "física", Kind: "text", Opening: "Abertura", Content: "Texto privado",
		Takeaways: []string{"um", "dois", "três"},
	}}
	runner := NewLearningRunner(generator, &fakeMailer{}, openLearningStore(t), []string{"física"})
	runner.SetLogger(func(format string, args ...any) { logs.WriteString(format + "\n") })
	runner.now = func() time.Time { return time.Date(2026, 8, 3, 18, 0, 0, 0, time.UTC) }
	runner.sleep = func(context.Context, time.Duration) error { return nil }

	if err := runner.RunSlot(context.Background(), "evening"); err != nil {
		t.Fatalf("RunSlot() error = %v", err)
	}
	if strings.Contains(logs.String(), "Segredo") || strings.Contains(logs.String(), "Texto privado") {
		t.Fatalf("logs contain lesson content: %s", logs.String())
	}
}

func TestLearningRunnerUsesPersistentHistoryAndDeduplicatesAcrossInstances(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "learning.sqlite3")
	generator := &fakeLessonGenerator{lesson: llm.Lesson{
		Title: "Título", Topic: "matemática", Kind: "text", Opening: "Abertura", Content: "Conteúdo",
		Takeaways: []string{"um", "dois", "três"},
	}}
	firstStore, err := storage.Open(ctx, path, 365)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	firstMailer := &fakeMailer{}
	first := NewLearningRunner(generator, firstMailer, firstStore, []string{"matemática", "física"})
	first.now = func() time.Time { return time.Date(2026, 8, 3, 7, 0, 0, 0, time.UTC) }
	first.sleep = func(context.Context, time.Duration) error { return nil }
	if err := first.RunSlot(ctx, "morning"); err != nil {
		t.Fatalf("first RunSlot() error = %v", err)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("first store close error = %v", err)
	}

	secondStore, err := storage.Open(ctx, path, 365)
	if err != nil {
		t.Fatalf("reopen storage error = %v", err)
	}
	defer secondStore.Close()
	secondMailer := &fakeMailer{}
	second := NewLearningRunner(generator, secondMailer, secondStore, []string{"matemática", "física"})
	second.now = first.now
	second.sleep = func(context.Context, time.Duration) error { return nil }
	if err := second.RunSlot(ctx, "morning"); err != nil {
		t.Fatalf("second RunSlot() error = %v", err)
	}
	if len(secondMailer.sent) != 0 {
		t.Fatalf("second mailer sent %d messages, want persistent duplicate guard", len(secondMailer.sent))
	}
}

func openLearningStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "learning.sqlite3"), 365)
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
