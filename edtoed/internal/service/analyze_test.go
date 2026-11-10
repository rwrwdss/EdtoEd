package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	pb "edtoed/api/proto"
	"edtoed/internal/auth"
	"edtoed/internal/features"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type storeStub struct {
	median float64
	count  int
}

func (s storeStub) SnapshotMedian(context.Context, string, *pb.StudentActivityRequest) (float64, int, error) {
	return s.median, s.count, nil
}

func platformContext(t *testing.T) context.Context {
	t.Helper()
	return organizationContext(t, "platform", strings.Repeat("b", 64))
}

func organizationContext(t *testing.T, org, token string) context.Context {
	t.Helper()
	registry := fmt.Sprintf(`[{"organization_id":%q,"sha256":"%x"}]`, org, sha256.Sum256([]byte(token)))
	keys, err := auth.Load(strings.NewReader(registry))
	if err != nil {
		t.Fatal(err)
	}
	ctx, ok := keys.Authenticate(t.Context(), []string{"Bearer " + token})
	if !ok {
		t.Fatal("expected authenticated context")
	}
	return ctx
}

type heldStore struct {
	entered chan struct{}
}

func (s heldStore) SnapshotMedian(ctx context.Context, org string, req *pb.StudentActivityRequest) (float64, int, error) {
	if org == "platform" && req.StudentId == "held" {
		close(s.entered)
		<-ctx.Done()
		return 0, 0, ctx.Err()
	}
	return 20, 3, nil
}

func TestOrganizationLimitSharesKeysAndReleasesOnCancel(t *testing.T) {
	store := heldStore{entered: make(chan struct{})}
	a := Analyzer{Store: store, Predictor: fixedRisk(.2), Limiter: NewOrganizationLimiter(1)}
	ctx, cancel := context.WithCancel(platformContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := a.AnalyzeStudentMetrics(ctx, &pb.StudentActivityRequest{
			StudentId: "held", TopicId: "t", CohortId: "c", WindowStart: 1, WindowEnd: 2,
		})
		done <- err
	}()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("first analysis did not reach storage")
	}

	req := &pb.StudentActivityRequest{StudentId: "s", TopicId: "t", CohortId: "c", WindowStart: 1, WindowEnd: 2}
	secondKey := organizationContext(t, "platform", strings.Repeat("c", 64))
	if _, err := a.AnalyzeStudentMetrics(secondKey, req); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("second key for the same organization: %v", err)
	}
	other := organizationContext(t, "other", strings.Repeat("d", 64))
	if _, err := a.AnalyzeStudentMetrics(other, req); err != nil {
		t.Fatalf("other organization: %v", err)
	}
	cancel()
	if err := <-done; status.Code(err) != codes.Canceled {
		t.Fatalf("held request: %v", err)
	}
	if _, err := a.AnalyzeStudentMetrics(secondKey, req); err != nil {
		t.Fatalf("slot was not released: %v", err)
	}
}

type capture struct{ v features.Vector }

func (p *capture) Predict(_ context.Context, v features.Vector) (float32, error) {
	p.v = v
	return .2, nil
}

type fixedRisk float32

func (p fixedRisk) Predict(context.Context, features.Vector) (float32, error) {
	return float32(p), nil
}
func TestInterventionThreshold(t *testing.T) {
	r := &pb.StudentActivityRequest{StudentId: "s", TopicId: "t", CohortId: "c", WindowStart: 1, WindowEnd: 2, ClicksCount: 30, TestScore: .5}
	a := Analyzer{Store: storeStub{20, 3}, Predictor: fixedRisk(interventionThreshold - .001)}
	ctx := platformContext(t)
	below, err := a.AnalyzeStudentMetrics(ctx, r)
	if err != nil || below.RequiresAction || below.ActionType != "KEEP_TRACK" {
		t.Fatalf("below: %v %v", below, err)
	}
	a.Predictor = fixedRisk(interventionThreshold)
	above, err := a.AnalyzeStudentMetrics(ctx, r)
	if err != nil || !above.RequiresAction || above.ActionType != "OFFER_HINT" {
		t.Fatalf("above: %v %v", above, err)
	}
}
func TestCohortNormalization(t *testing.T) {
	p := &capture{}
	a := Analyzer{Store: storeStub{20, 3}, Predictor: p}
	r := &pb.StudentActivityRequest{StudentId: "s", TopicId: "t", CohortId: "c", WindowStart: 1, WindowEnd: 2, ClicksCount: 30, TestScore: .5}
	ctx := platformContext(t)
	if _, err := a.AnalyzeStudentMetrics(ctx, r); err != nil {
		t.Fatal(err)
	}
	if p.v[0] != 1.5 {
		t.Fatal(p.v)
	}
	for _, s := range []storeStub{{0, 3}, {10, 1}} {
		a.Store = s
		if _, err := a.AnalyzeStudentMetrics(ctx, r); status.Code(err) != codes.FailedPrecondition {
			t.Fatal(err)
		}
	}
	r.CohortId = ""
	if _, err := a.AnalyzeStudentMetrics(ctx, r); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
