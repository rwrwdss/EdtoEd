package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "edtoed/api/proto"
	"edtoed/internal/httpapi"
	"edtoed/internal/ml"
	"edtoed/internal/service"
	"edtoed/internal/storage"

	"google.golang.org/grpc"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	modelPath := os.Getenv("MODEL_PATH")
	if modelPath == "" {
		modelPath = "models/edtoed_v1.onnx"
	}
	if err := ml.ModelFile(modelPath); err != nil {
		log.Fatalf("model file: %v", err)
	}
	store, err := storage.NewStorage(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	grpcAddr := os.Getenv("GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":50051"
	}
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatal(err)
	}
	server := grpc.NewServer()
	predictor, err := ml.NewRuntime(modelPath)
	if err != nil {
		log.Fatal(err)
	}
	defer predictor.Close()
	analyzer := &service.Analyzer{Store: store, Predictor: predictor}
	pb.RegisterEdtoEdServiceServer(server, analyzer)
	go func() {
		if err := server.Serve(listener); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}
	httpServer := &http.Server{Addr: httpAddr, Handler: httpapi.Handler{Analyzer: analyzer}.Routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server stopped: %v", err)
		}
	}()
	log.Printf("gRPC listening on %s; HTTP listening on %s", grpcAddr, httpAddr)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	server.GracefulStop()
}
