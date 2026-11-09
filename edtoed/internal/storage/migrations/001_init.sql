CREATE TABLE IF NOT EXISTS lms_anonymous_logs (
    id BIGSERIAL PRIMARY KEY,
    student_id TEXT NOT NULL,
    topic_id TEXT NOT NULL,
    clicks_count INTEGER NOT NULL CHECK (clicks_count >= 0),
    test_score REAL NOT NULL CHECK (test_score >= 0 AND test_score <= 1),
    deadline_delay REAL NOT NULL,
    is_trained BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS lms_anonymous_logs_topic_created_idx
    ON lms_anonymous_logs (topic_id, created_at);
