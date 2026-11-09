package service

import (
	"context"
	pb "edtoed/api/proto"
	"edtoed/internal/features"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

type storeStub struct {
	median float64
	count  int
}

func (s storeStub) SnapshotMedian(context.Context, *pb.StudentActivityRequest) (float64, int, error) {
	return s.median, s.count, nil
}

type capture struct{ v features.Vector }

func (p *capture) Predict(v features.Vector) (float32, error) { p.v = v; return .2, nil }
func TestCohortNormalization(t *testing.T) {
	p := &capture{}
	a := Analyzer{Store: storeStub{20, 3}, Predictor: p}
	r := &pb.StudentActivityRequest{StudentId: "s", TopicId: "t", CohortId: "c", WindowStart: 1, WindowEnd: 2, ClicksCount: 30, TestScore: .5}
	if _, err := a.AnalyzeStudentMetrics(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if p.v[0] != 1.5 {
		t.Fatal(p.v)
	}
	for _, s := range []storeStub{{0, 3}, {10, 1}} {
		a.Store = s
		if _, err := a.AnalyzeStudentMetrics(context.Background(), r); status.Code(err) != codes.FailedPrecondition {
			t.Fatal(err)
		}
	}
	r.CohortId = ""
	if _, err := a.AnalyzeStudentMetrics(context.Background(), r); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
