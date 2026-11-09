package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/lib/pq"
)

//go:embed migrations/001_init.sql
var initialMigration string

type Storage struct {
	db *sql.DB
}

func NewStorage(connStr string) (*Storage, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	if _, err := db.ExecContext(context.Background(), initialMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("create lms_anonymous_logs: %w", err)
	}
	return &Storage{db: db}, nil
}

// SaveLog сохраняет анонимный лог для последующего переобучения модели
func (s *Storage) SaveLog(uid, tid string, clicks int, score, delay float32) error {
	query := `INSERT INTO lms_anonymous_logs (student_id, topic_id, clicks_count, test_score, deadline_delay) VALUES ($1, $2, $3, $4, $5)`
	_, err := s.db.ExecContext(context.Background(), query, uid, tid, clicks, score, delay)
	return err
}

func (s *Storage) Close() error {
	return s.db.Close()
}
