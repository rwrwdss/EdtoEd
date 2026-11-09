CREATE TABLE IF NOT EXISTS activity_snapshots (
 cohort_id TEXT NOT NULL, topic_id TEXT NOT NULL,
 window_start BIGINT NOT NULL, window_end BIGINT NOT NULL CHECK(window_end > window_start),
 student_id TEXT NOT NULL, clicks_count INTEGER NOT NULL CHECK(clicks_count >= 0),
 test_score REAL NOT NULL, deadline_delay REAL NOT NULL,
 PRIMARY KEY(cohort_id,topic_id,window_start,window_end,student_id)
);
