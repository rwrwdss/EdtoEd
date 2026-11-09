package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	pb "edtoed/api/proto"
	"edtoed/internal/features"
	"edtoed/internal/ml"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Store interface {
	SnapshotMedian(context.Context, *pb.StudentActivityRequest) (float64, int, error)
}

type Analyzer struct {
	pb.UnimplementedEdtoEdServiceServer
	Store     Store
	Predictor ml.Predictor
}

func (a *Analyzer) AnalyzeStudentMetrics(ctx context.Context, req *pb.StudentActivityRequest) (*pb.AnalyticsResponse, error) {
	if req == nil || req.StudentId == "" || req.TopicId == "" || req.ClicksCount < 0 || math.IsNaN(float64(req.TestScore)) || req.TestScore < 0 || req.TestScore > 1 || math.IsNaN(float64(req.DeadlineDelay)) || math.IsInf(float64(req.DeadlineDelay), 0) {
		return nil, status.Error(codes.InvalidArgument, "invalid student activity")
	}
	if strings.TrimSpace(req.CohortId) == "" || req.WindowStart <= 0 || req.WindowEnd <= req.WindowStart || req.WindowEnd > time.Now().Unix() {
		return nil, status.Error(codes.InvalidArgument, "cohort and completed observation window required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	median, count, err := a.Store.SnapshotMedian(ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not save activity")
	}
	// The cohort median is not yet available. This fallback is explicit until
	// the aggregation layer is implemented.
	if count < 2 || median <= 0 || math.IsNaN(median) || math.IsInf(median, 0) {
		return nil, status.Error(codes.FailedPrecondition, "snapshot saved; need two students and positive median")
	}
	vector := features.Build(req.ClicksCount, median, req.TestScore, req.DeadlineDelay)
	risk, err := a.Predictor.Predict(vector)
	if errors.Is(err, ml.ErrInferenceUnavailable) {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "prediction failed")
	}
	if math.IsNaN(float64(risk)) || risk < 0 || risk > 1 {
		return nil, status.Error(codes.Internal, "invalid prediction")
	}
	action := "KEEP_TRACK"
	if risk >= 0.7 {
		action = "OFFER_HINT"
	}
	return &pb.AnalyticsResponse{StudentId: req.StudentId, BurnoutRisk: risk, RequiresAction: risk >= 0.7, ActionType: action}, nil
}
