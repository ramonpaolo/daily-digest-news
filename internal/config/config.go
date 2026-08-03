package config

import (
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                  string
	ScheduleTime          string
	LearningMorningTime   string
	LearningEveningTime   string
	Timezone              string
	Location              *time.Location
	TopStories            int
	LearningTopics        []string
	SQLitePath            string
	LearningRetentionDays int

	AIBaseURL string
	AIAPIKey  string
	AIModel   string

	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	EmailTo      string
}

var defaultLearningTopics = []string{
	"matemática",
	"história da matemática",
	"computação",
	"Go",
	"estruturas de dados e algoritmos",
	"PostgreSQL",
	"bancos de dados",
	"sistemas operacionais",
	"física",
}

func Load() (Config, error) {
	cfg := Config{
		Port:                valueOr("PORT", "8080"),
		ScheduleTime:        valueOr("SCHEDULE_TIME", "08:00"),
		LearningMorningTime: valueOr("LEARNING_MORNING_TIME", "07:00"),
		LearningEveningTime: valueOr("LEARNING_EVENING_TIME", "18:00"),
		Timezone:            valueOr("TIMEZONE", "America/Sao_Paulo"),
		AIBaseURL:           valueOr("ZENIFRA_AI_BASE_URL", "https://ai.zenifra.com/v1"),
		AIAPIKey:            os.Getenv("ZENIFRA_AI_API_KEY"),
		AIModel:             os.Getenv("ZENIFRA_AI_MODEL"),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPSecurity:        valueOr("SMTP_SECURITY", "starttls"),
		SMTPUsername:        strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		SMTPPassword:        compactSecret(os.Getenv("SMTP_PASSWORD")),
		SMTPFrom:            strings.TrimSpace(os.Getenv("SMTP_FROM")),
		EmailTo:             strings.TrimSpace(os.Getenv("EMAIL_TO")),
		LearningTopics:      parseTopics(valueOr("LEARNING_TOPICS", strings.Join(defaultLearningTopics, ","))),
		SQLitePath:          valueOr("SQLITE_PATH", "/data/daily-digest-news.sqlite3"),
	}

	for _, name := range []string{
		"ZENIFRA_AI_API_KEY", "ZENIFRA_AI_MODEL", "SMTP_HOST", "SMTP_USERNAME",
		"SMTP_PASSWORD", "SMTP_FROM", "EMAIL_TO",
	} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return Config{}, fmt.Errorf("required environment variable %s is missing", name)
		}
	}

	if _, err := time.Parse("15:04", cfg.ScheduleTime); err != nil {
		return Config{}, fmt.Errorf("invalid SCHEDULE_TIME: expected HH:MM")
	}
	if _, err := time.Parse("15:04", cfg.LearningMorningTime); err != nil {
		return Config{}, fmt.Errorf("invalid LEARNING_MORNING_TIME: expected HH:MM")
	}
	if _, err := time.Parse("15:04", cfg.LearningEveningTime); err != nil {
		return Config{}, fmt.Errorf("invalid LEARNING_EVENING_TIME: expected HH:MM")
	}
	if len(cfg.LearningTopics) == 0 {
		return Config{}, fmt.Errorf("invalid LEARNING_TOPICS: at least one topic is required")
	}
	retentionDays, err := strconv.Atoi(valueOr("LEARNING_RETENTION_DAYS", "365"))
	if err != nil || retentionDays < 0 {
		return Config{}, fmt.Errorf("invalid LEARNING_RETENTION_DAYS: expected a non-negative integer")
	}
	cfg.LearningRetentionDays = retentionDays
	if strings.TrimSpace(cfg.SQLitePath) == "" {
		return Config{}, fmt.Errorf("invalid SQLITE_PATH: path is required")
	}
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return Config{}, fmt.Errorf("invalid TIMEZONE: %w", err)
	}
	cfg.Location = location

	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("invalid PORT: expected 1-65535")
	}

	smtpPortText := valueOr("SMTP_PORT", "587")
	smtpPort, err := strconv.Atoi(smtpPortText)
	if err != nil || smtpPort < 1 || smtpPort > 65535 {
		return Config{}, fmt.Errorf("invalid SMTP_PORT: expected 1-65535")
	}
	cfg.SMTPPort = smtpPort

	if cfg.SMTPSecurity != "starttls" && cfg.SMTPSecurity != "tls" {
		return Config{}, fmt.Errorf("invalid SMTP_SECURITY: use starttls or tls")
	}
	if _, err := mail.ParseAddress(cfg.SMTPFrom); err != nil {
		return Config{}, fmt.Errorf("invalid SMTP_FROM address")
	}
	if _, err := mail.ParseAddress(cfg.EmailTo); err != nil {
		return Config{}, fmt.Errorf("invalid EMAIL_TO address")
	}

	topStories, err := strconv.Atoi(valueOr("TOP_STORIES", "10"))
	if err != nil || topStories < 1 || topStories > 20 {
		return Config{}, fmt.Errorf("invalid TOP_STORIES: expected 1-20")
	}
	cfg.TopStories = topStories

	return cfg, nil
}

func valueOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func compactSecret(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func parseTopics(value string) []string {
	seen := make(map[string]struct{})
	topics := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		topic := strings.Join(strings.Fields(raw), " ")
		if topic == "" {
			continue
		}
		key := strings.ToLower(topic)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		topics = append(topics, topic)
	}
	return topics
}
