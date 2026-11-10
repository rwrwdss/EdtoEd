package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	pb "edtoed/api/proto"

	_ "github.com/lib/pq"
)

//go:embed migrations/001_init.sql
var initialMigration string

//go:embed migrations/002_snapshots.sql
var snapshotMigration string

//go:embed migrations/003_organization_snapshots.sql
var organizationSnapshotMigration string

type Storage struct {
	db *sql.DB
}

func (s *Storage) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func NewStorage(ctx context.Context, connStr string) (*Storage, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	if _, err := db.ExecContext(ctx, initialMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("create lms_anonymous_logs: %w", err)
	}
	if _, err := db.ExecContext(ctx, snapshotMigration); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, organizationSnapshotMigration); err != nil {
		db.Close()
		return nil, fmt.Errorf("create organization snapshots: %w", err)
	}
	return &Storage{db: db}, nil
}

// SnapshotMedian upserts one aggregate per student and serializes cohort updates.
func (s *Storage) SnapshotMedian(ctx context.Context, org string, r *pb.StudentActivityRequest) (float64, int, error) {
	if org == "" {
		return 0, 0, errors.New("organization is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	// Serialize writes to this window so the median includes the just-saved
	// snapshot and cannot race another update in the same cohort.
	scope := []any{org, r.CohortId, r.TopicId, r.WindowStart, r.WindowEnd}
	key, _ := json.Marshal(scope)
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, string(key)); err != nil {
		return 0, 0, err
	}
	args := append(scope, r.StudentId, r.ClicksCount, r.TestScore, r.DeadlineDelay)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO organization_activity_snapshots
			(organization_id, cohort_id, topic_id, window_start, window_end, student_id, clicks_count, test_score, deadline_delay)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (organization_id, cohort_id, topic_id, window_start, window_end, student_id)
		DO UPDATE SET clicks_count=EXCLUDED.clicks_count, test_score=EXCLUDED.test_score, deadline_delay=EXCLUDED.deadline_delay`, args...)
	if err != nil {
		return 0, 0, err
	}
	var median float64
	var count int
	err = tx.QueryRowContext(ctx, `
		SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY clicks_count), count(*)
		FROM organization_activity_snapshots
		WHERE organization_id=$1 AND cohort_id=$2 AND topic_id=$3 AND window_start=$4 AND window_end=$5`, scope...).Scan(&median, &count)
	if err != nil {
		return 0, 0, err
	}
	err = tx.Commit()
	return median, count, err
}

func (s *Storage) Close() error {
	return s.db.Close()
}
