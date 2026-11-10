package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "edtoed/api/proto"
	"edtoed/internal/auth"
	"edtoed/internal/httpapi"
	"edtoed/internal/ml"
	"edtoed/internal/service"
	"edtoed/internal/storage"

	"google.golang.org/grpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	keys, err := loadKeys(os.Getenv("EDTOED_KEYS_FILE"))
	if err != nil {
		return err
	}
	predictor, err := ml.NewRuntime(ctx, envOr("MODEL_PATH", "models/edtoed_v1.onnx"))
	if err != nil {
		return fmt.Errorf("initialize model: %w", err)
	}
	defer predictor.Close()
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := storage.NewStorage(startup, dsn)
	cancel()
	if err != nil {
		return err
	}
	defer store.Close()

	grpcListener, err := net.Listen("tcp", envOr("GRPC_ADDR", ":50051"))
	if err != nil {
		return fmt.Errorf("bind gRPC: %w", err)
	}
	defer grpcListener.Close()
	httpListener, err := net.Listen("tcp", envOr("HTTP_ADDR", ":8080"))
	if err != nil {
		return fmt.Errorf("bind HTTP: %w", err)
	}
	defer httpListener.Close()

	analyzer := &service.Analyzer{Store: store, Predictor: predictor}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(keys.Unary))
	pb.RegisterEdtoEdServiceServer(grpcServer, analyzer)
	httpServer := &http.Server{
		Handler:           httpapi.Handler{Analyzer: analyzer, Keys: keys}.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("gRPC listening on %s; HTTP listening on %s", grpcListener.Addr(), httpListener.Addr())
	return serve(ctx, httpServer, httpListener, grpcServer, grpcListener)
}

func loadKeys(path string) (*auth.Keys, error) {
	if path == "" {
		return nil, errors.New("EDTOED_KEYS_FILE is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open key registry: %w", err)
	}
	defer file.Close()
	return auth.Load(file)
}

func serve(ctx context.Context, httpServer *http.Server, httpListener net.Listener, grpcServer *grpc.Server, grpcListener net.Listener) error {
	defer httpListener.Close()
	defer grpcListener.Close()
	results := make(chan error, 2)
	go func() {
		err := httpServer.Serve(httpListener)
		if !errors.Is(err, http.ErrServerClosed) {
			results <- fmt.Errorf("serve HTTP: %w", err)
		}
	}()
	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			results <- fmt.Errorf("serve gRPC: %w", err)
		}
	}()

	var serveErr error
	select {
	case serveErr = <-results:
	case <-ctx.Done():
	}
	// Both listeners share the grace period. A stalled RPC cannot delay HTTP
	// shutdown or keep the worker alive indefinitely.
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	grpcDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcDone)
	}()
	if err := httpServer.Shutdown(drain); err != nil {
		_ = httpServer.Close()
	}
	select {
	case <-grpcDone:
	case <-drain.Done():
		grpcServer.Stop()
	}
	return serveErr
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
