package storage

import (
	"context"
	pb "edtoed/api/proto"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSnapshotMedianIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	s, err := NewStorage(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cohort := fmt.Sprintf("test-%d", time.Now().UnixNano())
	defer s.db.Exec(`DELETE FROM activity_snapshots WHERE cohort_id=$1`, cohort)
	r := &pb.StudentActivityRequest{CohortId: cohort, TopicId: "t", WindowStart: 1, WindowEnd: 2, StudentId: "a", ClicksCount: 10}
	check := func(want float64, n int) {
		t.Helper()
		m, c, e := s.SnapshotMedian(context.Background(), r)
		if e != nil || m != want || c != n {
			t.Fatalf("median %v count %d err %v; want %v/%d", m, c, e, want, n)
		}
	}
	check(10, 1)
	r.StudentId = "b"
	r.ClicksCount = 30
	check(20, 2)
	r.ClicksCount = 50
	check(30, 2)
	r.StudentId = "c"
	r.ClicksCount = 0
	check(10, 3)
	r.WindowEnd = 3
	check(0, 1)
	r.TopicId = "other"
	r.ClicksCount = 5
	check(5, 1)
}
