package config

import (
	"strings"
	"testing"
)

func TestLoadUsesSafeDefaults(t *testing.T) {
	setRequired(t)
	t.Setenv("SMTP_PASSWORD", " abcd efgh ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.ScheduleTime != "08:00" {
		t.Fatalf("ScheduleTime = %q, want 08:00", cfg.ScheduleTime)
	}
	if cfg.LearningMorningTime != "07:00" || cfg.LearningEveningTime != "18:00" {
		t.Fatalf("learning times = %q/%q, want 07:00/18:00", cfg.LearningMorningTime, cfg.LearningEveningTime)
	}
	if cfg.SystemDesignTime != "06:30" || cfg.EngineeringBlogsTime != "20:00" || len(cfg.EngineeringBlogSources) != 5 {
		t.Fatalf("new schedules/sources = %q/%q/%#v", cfg.SystemDesignTime, cfg.EngineeringBlogsTime, cfg.EngineeringBlogSources)
	}
	if len(cfg.LearningTopics) != 9 || cfg.LearningTopics[0] != "matemática" || cfg.LearningTopics[1] != "história da matemática" {
		t.Fatalf("LearningTopics = %#v, want default topic list", cfg.LearningTopics)
	}
	if cfg.Timezone != "America/Sao_Paulo" {
		t.Fatalf("Timezone = %q, want America/Sao_Paulo", cfg.Timezone)
	}
	if cfg.TopStories != 10 {
		t.Fatalf("TopStories = %d, want 10", cfg.TopStories)
	}
	if cfg.SMTPPassword != "abcdefgh" {
		t.Fatalf("SMTPPassword = %q, want compacted secret", cfg.SMTPPassword)
	}
	if cfg.AIBaseURL != "https://ai.zenifra.com/v1" {
		t.Fatalf("AIBaseURL = %q, want Zenifra default", cfg.AIBaseURL)
	}
	if cfg.SQLitePath != "/data/daily-digest-news.sqlite3" {
		t.Fatalf("SQLitePath = %q, want default persistent path", cfg.SQLitePath)
	}
	if cfg.LearningRetentionDays != 365 {
		t.Fatalf("LearningRetentionDays = %d, want 365", cfg.LearningRetentionDays)
	}
}

func TestLoadAcceptsLearningOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("LEARNING_MORNING_TIME", "06:30")
	t.Setenv("LEARNING_EVENING_TIME", "19:15")
	t.Setenv("LEARNING_TOPICS", " matemática, física, matemática ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LearningMorningTime != "06:30" || cfg.LearningEveningTime != "19:15" {
		t.Fatalf("learning times = %q/%q", cfg.LearningMorningTime, cfg.LearningEveningTime)
	}
	if strings.Join(cfg.LearningTopics, ",") != "matemática,física" {
		t.Fatalf("LearningTopics = %#v, want trimmed unique list", cfg.LearningTopics)
	}
}

func TestLoadRejectsInvalidLearningTime(t *testing.T) {
	setRequired(t)
	t.Setenv("LEARNING_EVENING_TIME", "25:00")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "LEARNING_EVENING_TIME") {
		t.Fatalf("Load() error = %v, want invalid learning time", err)
	}
}

func TestLoadAcceptsSQLiteOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("SQLITE_PATH", "/mnt/history/lessons.db")
	t.Setenv("LEARNING_RETENTION_DAYS", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SQLitePath != "/mnt/history/lessons.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.LearningRetentionDays != 0 {
		t.Fatalf("LearningRetentionDays = %d, want unlimited", cfg.LearningRetentionDays)
	}
}

func TestLoadRejectsNegativeSQLiteRetention(t *testing.T) {
	setRequired(t)
	t.Setenv("LEARNING_RETENTION_DAYS", "-1")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "LEARNING_RETENTION_DAYS") {
		t.Fatalf("Load() error = %v, want invalid retention", err)
	}
}

func TestLoadRejectsMissingRequiredEnvironment(t *testing.T) {
	setRequired(t)
	t.Setenv("SMTP_PASSWORD", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SMTP_PASSWORD") {
		t.Fatalf("Load() error = %v, want missing SMTP_PASSWORD", err)
	}
}

func TestLoadRejectsInvalidSchedule(t *testing.T) {
	setRequired(t)
	t.Setenv("SCHEDULE_TIME", "25:61")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SCHEDULE_TIME") {
		t.Fatalf("Load() error = %v, want invalid SCHEDULE_TIME", err)
	}
}

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("ZENIFRA_AI_API_KEY", "test-key")
	t.Setenv("ZENIFRA_AI_MODEL", "test-model")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "sender@example.test")
	t.Setenv("SMTP_PASSWORD", "password")
	t.Setenv("SMTP_FROM", "sender@example.test")
	t.Setenv("EMAIL_TO", "receiver@example.test")
}
