package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/llm"
	_ "modernc.org/sqlite"
)

type Store struct {
	db            *sql.DB
	retentionDays int
}

type LessonClaim struct {
	ID          int64
	Topic       string
	AlreadySent bool
}

type ContentClaim struct {
	ID          int64
	AlreadySent bool
}

type HistoryLesson struct {
	DateKey string
	Slot    string
	Topic   string
	Subject string
	Kind    string
	Title   string
}

func Open(ctx context.Context, path string, retentionDays int) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	if retentionDays < 0 {
		return nil, errors.New("sqlite retention days must be non-negative")
	}
	if parent := filepath.Dir(path); parent != "." {
		if err := os.MkdirAll(parent, 0o750); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, retentionDays: retentionDays}
	if err := store.configure(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("restrict sqlite permissions: %w", err)
	}
	return store, nil
}

func (s *Store) configure(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA synchronous = NORMAL`); err != nil {
		return fmt.Errorf("configure sqlite synchronous mode: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("configure sqlite busy timeout: %w", err)
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping sqlite: %w", err)
	}
	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create sqlite migrations table: %w", err)
	}
	var version int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if version >= 4 {
		return nil
	}
	if version < 1 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin sqlite migration: %w", err)
		}
		const schema = `
CREATE TABLE interests (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	active INTEGER NOT NULL DEFAULT 1,
	first_seen_at TEXT NOT NULL,
	last_used_at TEXT
);

CREATE TABLE learning_lessons (
	id INTEGER PRIMARY KEY,
	date_key TEXT NOT NULL,
	slot TEXT NOT NULL,
	slot_key TEXT UNIQUE,
	topic TEXT NOT NULL,
	request_json TEXT NOT NULL,
	title TEXT,
	kind TEXT,
	opening TEXT,
	content TEXT,
	answer TEXT,
	takeaways_json TEXT,
	status TEXT NOT NULL,
	phase TEXT,
	error TEXT,
	attempts INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	generated_at TEXT,
	sent_at TEXT
);

CREATE INDEX learning_lessons_history_idx ON learning_lessons(status, sent_at DESC);
CREATE INDEX learning_lessons_created_idx ON learning_lessons(created_at);
`
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply sqlite schema: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record sqlite migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite migration: %w", err)
		}
		version = 1
	}
	if version < 2 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin sqlite subject migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE learning_lessons ADD COLUMN subject TEXT`); err != nil {
			tx.Rollback()
			return fmt.Errorf("add lesson subject column: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE learning_lessons SET subject = title WHERE subject IS NULL OR trim(subject) = ''`); err != nil {
			tx.Rollback()
			return fmt.Errorf("backfill lesson subjects: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(2, ?)`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record sqlite subject migration: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite subject migration: %w", err)
		}
	}
	if version < 3 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin system design migration: %w", err)
		}
		_, err = tx.ExecContext(ctx, `CREATE TABLE system_design_lessons (id INTEGER PRIMARY KEY, date_key TEXT NOT NULL, slot_key TEXT UNIQUE, subject TEXT, content_json TEXT, status TEXT NOT NULL, error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, sent_at TEXT)`)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("create system design table: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(3, ?)`, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		version = 3
	}
	if version < 4 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin blog migration: %w", err)
		}
		_, err = tx.ExecContext(ctx, `CREATE TABLE engineering_blog_runs (id INTEGER PRIMARY KEY, week_key TEXT NOT NULL UNIQUE, status TEXT NOT NULL, content_json TEXT, error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, sent_at TEXT); CREATE TABLE engineering_blog_items (id INTEGER PRIMARY KEY, run_id INTEGER NOT NULL, item_key TEXT NOT NULL UNIQUE, source_name TEXT NOT NULL, title TEXT NOT NULL, url TEXT NOT NULL, published_at TEXT, FOREIGN KEY(run_id) REFERENCES engineering_blog_runs(id))`)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("create blog tables: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(4, ?)`, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) BeginSystemDesign(ctx context.Context, dateKey string, now time.Time) (ContentClaim, error) {
	key := strings.TrimSpace(dateKey)
	if key == "" {
		return ContentClaim{}, errors.New("system design date is required")
	}
	var id int64
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT id,status FROM system_design_lessons WHERE slot_key=?`, key).Scan(&id, &status)
	if err == nil {
		return ContentClaim{ID: id, AlreadySent: status == "sent"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ContentClaim{}, err
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	err = s.db.QueryRowContext(ctx, `INSERT INTO system_design_lessons(date_key,slot_key,status,created_at,updated_at) VALUES(?,?,?, ?,?) RETURNING id`, key, key, "generating", ts, ts).Scan(&id)
	if err != nil {
		return ContentClaim{}, fmt.Errorf("claim system design: %w", err)
	}
	return ContentClaim{ID: id}, nil
}

func (s *Store) RecentSystemDesignSubjects(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT subject FROM system_design_lessons WHERE status='sent' AND subject IS NOT NULL ORDER BY sent_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveSystemDesign(ctx context.Context, id int64, subject, content string, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE system_design_lessons SET subject=?,content_json=?,status='email_pending',updated_at=? WHERE id=?`, subject, content, ts, id)
	return err
}
func (s *Store) MarkSystemDesignSent(ctx context.Context, id int64, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE system_design_lessons SET status='sent',sent_at=?,updated_at=? WHERE id=?`, ts, ts, id)
	return err
}
func (s *Store) MarkSystemDesignFailed(ctx context.Context, id int64, phase string, failure error, now time.Time) error {
	msg := "unknown"
	if failure != nil {
		msg = failure.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE system_design_lessons SET status='failed',error=?,updated_at=? WHERE id=?`, phase+": "+msg, now.UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Store) BeginEngineeringBlogRun(ctx context.Context, weekKey string, now time.Time) (ContentClaim, error) {
	var id int64
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT id,status FROM engineering_blog_runs WHERE week_key=?`, weekKey).Scan(&id, &status)
	if err == nil {
		return ContentClaim{ID: id, AlreadySent: status == "sent"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ContentClaim{}, err
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	err = s.db.QueryRowContext(ctx, `INSERT INTO engineering_blog_runs(week_key,status,created_at,updated_at) VALUES(?,?,?,?) RETURNING id`, weekKey, "generating", ts, ts).Scan(&id)
	return ContentClaim{ID: id}, err
}
func (s *Store) SentEngineeringBlogItems(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_key FROM engineering_blog_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}
func (s *Store) SaveEngineeringBlogRun(ctx context.Context, id int64, content string, items []struct{ Key, Source, Title, URL, Published string }, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `UPDATE engineering_blog_runs SET content_json=?,status='email_pending',updated_at=? WHERE id=?`, content, ts, id); err != nil {
		tx.Rollback()
		return err
	}
	for _, item := range items {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO engineering_blog_items(run_id,item_key,source_name,title,url,published_at) VALUES(?,?,?,?,?,?)`, id, item.Key, item.Source, item.Title, item.URL, item.Published); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) MarkEngineeringBlogSent(ctx context.Context, id int64, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE engineering_blog_runs SET status='sent',sent_at=?,updated_at=? WHERE id=?`, ts, ts, id)
	return err
}
func (s *Store) MarkEngineeringBlogFailed(ctx context.Context, id int64, phase string, failure error, now time.Time) error {
	msg := "unknown"
	if failure != nil {
		msg = failure.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE engineering_blog_runs SET status='failed',error=?,updated_at=? WHERE id=?`, phase+": "+msg, now.UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Store) SyncInterests(ctx context.Context, topics []string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin interests sync: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE interests SET active = 0`); err != nil {
		tx.Rollback()
		return fmt.Errorf("deactivate old interests: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	seen := make(map[string]struct{}, len(topics))
	for _, raw := range topics {
		topic := strings.Join(strings.Fields(raw), " ")
		key := strings.ToLower(topic)
		if topic == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO interests(name, active, first_seen_at) VALUES(?, 1, ?) ON CONFLICT(name) DO UPDATE SET active = 1`, topic, timestamp); err != nil {
			tx.Rollback()
			return fmt.Errorf("upsert interest: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit interests sync: %w", err)
	}
	return nil
}

func (s *Store) BeginLesson(ctx context.Context, dateKey, slot, requestJSON string, now time.Time) (LessonClaim, error) {
	dateKey = strings.TrimSpace(dateKey)
	slot = strings.TrimSpace(slot)
	if dateKey == "" || slot == "" {
		return LessonClaim{}, errors.New("lesson date and slot are required")
	}
	if strings.TrimSpace(requestJSON) == "" {
		requestJSON = `{}`
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LessonClaim{}, fmt.Errorf("begin lesson claim: %w", err)
	}
	var existingID int64
	var existingStatus string
	slotKey := dateKey + ":" + slot
	if slot == "startup" {
		slotKey = ""
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id, status FROM learning_lessons WHERE slot_key = ?`, slotKey).Scan(&existingID, &existingStatus)
		if err == nil && existingStatus == "sent" {
			tx.Rollback()
			return LessonClaim{ID: existingID, AlreadySent: true}, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			tx.Rollback()
			return LessonClaim{}, fmt.Errorf("check lesson slot: %w", err)
		}
	}
	var topic string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM interests WHERE active = 1 ORDER BY last_used_at IS NOT NULL, last_used_at, id LIMIT 1`).Scan(&topic); err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return LessonClaim{}, errors.New("no active learning interests")
		}
		return LessonClaim{}, fmt.Errorf("choose learning interest: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	if existingID != 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE learning_lessons SET topic = ?, request_json = ?, status = 'generating', phase = 'llm', error = NULL, attempts = attempts + 1, updated_at = ?, generated_at = NULL, sent_at = NULL WHERE id = ?`, topic, requestJSON, timestamp, existingID); err != nil {
			tx.Rollback()
			return LessonClaim{}, fmt.Errorf("reclaim lesson: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE interests SET last_used_at = ? WHERE name = ?`, timestamp, topic); err != nil {
			tx.Rollback()
			return LessonClaim{}, fmt.Errorf("update interest usage: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return LessonClaim{}, fmt.Errorf("commit lesson reclaim: %w", err)
		}
		return LessonClaim{ID: existingID, Topic: topic}, nil
	}
	var id int64
	if slotKey == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO learning_lessons(date_key, slot, topic, request_json, status, phase, created_at, updated_at) VALUES(?, ?, ?, ?, 'generating', 'llm', ?, ?) RETURNING id`, dateKey, slot, topic, requestJSON, timestamp, timestamp).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `INSERT INTO learning_lessons(date_key, slot, slot_key, topic, request_json, status, phase, created_at, updated_at) VALUES(?, ?, ?, ?, ?, 'generating', 'llm', ?, ?) RETURNING id`, dateKey, slot, slotKey, topic, requestJSON, timestamp, timestamp).Scan(&id)
	}
	if err != nil {
		tx.Rollback()
		return LessonClaim{}, fmt.Errorf("insert lesson claim: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE interests SET last_used_at = ? WHERE name = ?`, timestamp, topic); err != nil {
		tx.Rollback()
		return LessonClaim{}, fmt.Errorf("update interest usage: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return LessonClaim{}, fmt.Errorf("commit lesson claim: %w", err)
	}
	return LessonClaim{ID: id, Topic: topic}, nil
}

func (s *Store) SaveGenerated(ctx context.Context, id int64, lesson llm.Lesson, now time.Time) error {
	takeaways, err := json.Marshal(lesson.Takeaways)
	if err != nil {
		return fmt.Errorf("encode lesson takeaways: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE learning_lessons SET title = ?, subject = ?, kind = ?, opening = ?, content = ?, answer = ?, takeaways_json = ?, status = 'email_pending', phase = 'smtp', error = NULL, generated_at = ?, updated_at = ? WHERE id = ?`, lesson.Title, lesson.Subject, lesson.Kind, lesson.Opening, lesson.Content, lesson.Answer, string(takeaways), timestamp, timestamp, id)
	if err != nil {
		return fmt.Errorf("save generated lesson: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("save generated lesson affected %d rows", affected)
	}
	return nil
}

func (s *Store) UpdateRequest(ctx context.Context, id int64, requestJSON string, now time.Time) error {
	if strings.TrimSpace(requestJSON) == "" {
		requestJSON = `{}`
	}
	result, err := s.db.ExecContext(ctx, `UPDATE learning_lessons SET request_json = ?, updated_at = ? WHERE id = ?`, requestJSON, now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("save lesson request: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("save lesson request affected %d rows", affected)
	}
	return nil
}

func (s *Store) MarkSent(ctx context.Context, id int64, sentAt time.Time) error {
	timestamp := sentAt.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE learning_lessons SET status = 'sent', phase = 'smtp', error = NULL, sent_at = ?, updated_at = ? WHERE id = ?`, timestamp, timestamp, id)
	if err != nil {
		return fmt.Errorf("mark lesson sent: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("mark lesson sent affected %d rows", affected)
	}
	return nil
}

func (s *Store) MarkFailed(ctx context.Context, id int64, phase string, failure error, now time.Time) error {
	message := "unknown failure"
	if failure != nil {
		message = strings.Join(strings.Fields(failure.Error()), " ")
		if len(message) > 240 {
			message = message[:240] + "…"
		}
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `UPDATE learning_lessons SET status = 'failed', phase = ?, error = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(phase), message, timestamp, id)
	if err != nil {
		return fmt.Errorf("mark lesson failed: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("mark lesson failed affected %d rows", affected)
	}
	return nil
}

func (s *Store) RecentLessons(ctx context.Context, limit int) ([]HistoryLesson, error) {
	if limit < 1 {
		return []HistoryLesson{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT date_key, slot, topic, subject, kind, title FROM learning_lessons WHERE status = 'sent' ORDER BY sent_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent lessons: %w", err)
	}
	defer rows.Close()
	history := make([]HistoryLesson, 0, limit)
	for rows.Next() {
		var lesson HistoryLesson
		if err := rows.Scan(&lesson.DateKey, &lesson.Slot, &lesson.Topic, &lesson.Subject, &lesson.Kind, &lesson.Title); err != nil {
			return nil, fmt.Errorf("scan recent lesson: %w", err)
		}
		history = append(history, lesson)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent lessons: %w", err)
	}
	return history, nil
}

func (s *Store) Cleanup(ctx context.Context, now time.Time) error {
	if s.retentionDays == 0 {
		return nil
	}
	cutoff := now.UTC().Add(-time.Duration(s.retentionDays) * 24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM learning_lessons WHERE created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("cleanup old lessons: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM system_design_lessons WHERE created_at < ?`, cutoff); err != nil {
		return fmt.Errorf("cleanup old system design lessons: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM engineering_blog_items WHERE run_id IN (SELECT id FROM engineering_blog_runs WHERE created_at < ?); DELETE FROM engineering_blog_runs WHERE created_at < ?`, cutoff, cutoff); err != nil {
		return fmt.Errorf("cleanup old engineering blog history: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
