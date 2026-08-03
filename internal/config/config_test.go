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
