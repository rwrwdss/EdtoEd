package storage

import (
	"context"
	"database/sql"
	pb "edtoed/api/proto"
	_ "embed"
	"encoding/json"
	"fmt"

	_ "github.com/lib/pq"
)

//go:embed migrations/001_init.sql
var initialMigration string

//go:embed migrations/002_snapshots.sql
var snapshotMigration string

type Storage struct {
	db *sql.DB
}

func (s *Storage) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

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
	if _, err := db.ExecContext(context.Background(), snapshotMigration); err != nil {
		db.Close()
		return nil, err
	}
	return &Storage{db: db}, nil
}

// SnapshotMedian upserts one aggregate per student and serializes cohort updates.
func (s *Storage) SnapshotMedian(ctx context.Context, r *pb.StudentActivityRequest) (float64, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	key, _ := json.Marshal([]any{r.CohortId, r.TopicId, r.WindowStart, r.WindowEnd})
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, string(key)); err != nil {
		return 0, 0, err
	}
	args := []any{r.CohortId, r.TopicId, r.WindowStart, r.WindowEnd, r.StudentId, r.ClicksCount, r.TestScore, r.DeadlineDelay}
	_, err = tx.ExecContext(ctx, `INSERT INTO activity_snapshots VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(cohort_id,topic_id,window_start,window_end,student_id) DO UPDATE SET clicks_count=EXCLUDED.clicks_count,test_score=EXCLUDED.test_score,deadline_delay=EXCLUDED.deadline_delay`, args...)
	if err != nil {
		return 0, 0, err
	}
	var median float64
	var count int
	err = tx.QueryRowContext(ctx, `SELECT percentile_cont(0.5) WITHIN GROUP(ORDER BY clicks_count),count(*) FROM activity_snapshots WHERE cohort_id=$1 AND topic_id=$2 AND window_start=$3 AND window_end=$4`, args[:4]...).Scan(&median, &count)
	if err != nil {
		return 0, 0, err
	}
	err = tx.Commit()
	return median, count, err
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
