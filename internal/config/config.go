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
	Port         string
	ScheduleTime string
	Timezone     string
	Location     *time.Location
	TopStories   int

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

func Load() (Config, error) {
	cfg := Config{
		Port:         valueOr("PORT", "8080"),
		ScheduleTime: valueOr("SCHEDULE_TIME", "08:00"),
		Timezone:     valueOr("TIMEZONE", "America/Sao_Paulo"),
		AIBaseURL:    valueOr("ZENIFRA_AI_BASE_URL", "https://ai.zenifra.com/v1"),
		AIAPIKey:     os.Getenv("ZENIFRA_AI_API_KEY"),
		AIModel:      os.Getenv("ZENIFRA_AI_MODEL"),
		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPSecurity: valueOr("SMTP_SECURITY", "starttls"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),
		EmailTo:      os.Getenv("EMAIL_TO"),
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
