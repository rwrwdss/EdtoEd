package service

import (
	"context"
	"errors"
	"math"

	pb "edtoed/api/proto"
	"edtoed/internal/features"
	"edtoed/internal/ml"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Store interface {
	SaveLog(string, string, int, float32, float32) error
}

type Analyzer struct {
	pb.UnimplementedEdtoEdServiceServer
	Store     Store
	Predictor ml.Predictor
}

func (a *Analyzer) AnalyzeStudentMetrics(_ context.Context, req *pb.StudentActivityRequest) (*pb.AnalyticsResponse, error) {
	if req == nil || req.StudentId == "" || req.TopicId == "" || req.ClicksCount < 0 || math.IsNaN(float64(req.TestScore)) || req.TestScore < 0 || req.TestScore > 1 || math.IsNaN(float64(req.DeadlineDelay)) || math.IsInf(float64(req.DeadlineDelay), 0) {
		return nil, status.Error(codes.InvalidArgument, "invalid student activity")
	}
	if err := a.Store.SaveLog(req.StudentId, req.TopicId, int(req.ClicksCount), req.TestScore, req.DeadlineDelay); err != nil {
		return nil, status.Error(codes.Internal, "could not save activity")
	}
	// The cohort median is not yet available. This fallback is explicit until
	// the aggregation layer is implemented.
	vector := features.Build(req.ClicksCount, 1, req.TestScore, req.DeadlineDelay)
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
