package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/config"
	"github.com/ramonpaolo/daily-digest-news/internal/email"
	"github.com/ramonpaolo/daily-digest-news/internal/fetch"
	"github.com/ramonpaolo/daily-digest-news/internal/health"
	"github.com/ramonpaolo/daily-digest-news/internal/job"
	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	"github.com/ramonpaolo/daily-digest-news/internal/news"
	"github.com/ramonpaolo/daily-digest-news/internal/scheduler"
)

const (
	hackerNewsAPI      = "https://hacker-news.firebaseio.com"
	generalHTTPTimeout = 20 * time.Second
	aiHTTPTimeout      = 5 * time.Minute
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("component=app event=stopped error=%q", safeError(err))
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	command := "serve"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		command = args[0]
	}
	log.Printf("component=app event=config_loaded command=%s port=%s schedule=%s timezone=%s top_stories=%d ai_endpoint=%s ai_model=%s smtp_host=%s smtp_port=%d smtp_security=%s smtp_from_configured=%t email_to_configured=%t", command, cfg.Port, cfg.ScheduleTime, cfg.Timezone, cfg.TopStories, endpointLabel(cfg.AIBaseURL), cfg.AIModel, smtpHostLabel(cfg.SMTPHost), cfg.SMTPPort, cfg.SMTPSecurity, strings.TrimSpace(cfg.SMTPFrom) != "", strings.TrimSpace(cfg.EmailTo) != "")
	httpClient, aiHTTPClient := newHTTPClients()
	runner := job.NewRunner(
		news.NewClient(httpClient, hackerNewsAPI),
		fetch.NewFetcher(httpClient, nil),
		llm.NewClient(aiHTTPClient, cfg.AIBaseURL, cfg.AIAPIKey, cfg.AIModel),
		email.NewSMTPMailer(cfg),
	)
	runner.SetLimit(cfg.TopStories)
	runner.SetLocation(cfg.Location)

	switch command {
	case "run-once":
		return runner.RunOnce(context.Background())
	case "serve":
		return serve(cfg, runner)
	default:
		return fmt.Errorf("unknown command %q; use serve or run-once", command)
	}
}

func newHTTPClients() (*http.Client, *http.Client) {
	return &http.Client{Timeout: generalHTTPTimeout}, &http.Client{Timeout: aiHTTPTimeout}
}

func serve(cfg config.Config, runner *job.Runner) error {
	hour, minute, err := parseSchedule(cfg.ScheduleTime)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           health.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("component=health event=server_start address=0.0.0.0:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("component=health event=server_failed error=%q", safeError(err))
			serverErrors <- err
		}
	}()
	startupDone := startStartupDigest(ctx, runner)
	go func() {
		<-startupDone
		_ = scheduler.New(runner, cfg.Location, hour, minute).Run(ctx)
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

type startupRunner interface {
	RunOnce(context.Context) error
}

func startStartupDigest(ctx context.Context, runner startupRunner) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		started := time.Now()
		log.Printf("component=startup event=digest_start")
		if err := runner.RunOnce(ctx); err != nil {
			log.Printf("component=startup event=digest_failed duration_ms=%d error=%q", time.Since(started).Milliseconds(), safeError(err))
			return
		}
		log.Printf("component=startup event=digest_success duration_ms=%d", time.Since(started).Milliseconds())
	}()
	return done
}

func endpointLabel(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "invalid"
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.Path
}

func smtpHostLabel(raw string) string {
	parsed, err := url.Parse("//" + strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return "invalid"
	}
	return parsed.Hostname()
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(strings.Fields(err.Error()), " ")
	if len(value) > 240 {
		return value[:240] + "…"
	}
	return value
}

func parseSchedule(value string) (int, int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid schedule %q", value)
	}
	return parsed.Hour(), parsed.Minute(), nil
}