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

// Selected on the 2014B threshold subset (F2, precision >= 0.6).
const interventionThreshold float32 = 0.43382675

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
		if ctx.Err() != nil {
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		return nil, status.Error(codes.Internal, "could not save activity")
	}
	if count < 2 || median <= 0 || math.IsNaN(median) || math.IsInf(median, 0) {
		return nil, status.Error(codes.FailedPrecondition, "snapshot saved; need two students and positive median")
	}
	vector := features.Build(req.ClicksCount, median, req.TestScore, req.DeadlineDelay)
	risk, err := a.Predictor.Predict(ctx, vector)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, status.FromContextError(err).Err()
	}
	if errors.Is(err, ml.ErrInferenceUnavailable) {
		return nil, status.Error(codes.Unavailable, "model inference is unavailable")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "prediction failed")
	}
	if math.IsNaN(float64(risk)) || risk < 0 || risk > 1 {
		return nil, status.Error(codes.Internal, "invalid prediction")
	}
	action := "KEEP_TRACK"
	if risk >= interventionThreshold {
		action = "OFFER_HINT"
	}
	return &pb.AnalyticsResponse{StudentId: req.StudentId, BurnoutRisk: risk, RequiresAction: risk >= interventionThreshold, ActionType: action}, nil
}
