package main

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"

	"google.golang.org/grpc"
)

func TestServeStopsOtherListenerOnFailure(t *testing.T) {
	for _, failedProtocol := range []string{"HTTP", "gRPC"} {
		t.Run(failedProtocol, func(t *testing.T) {
			failure := errors.New("listener failed")
			httpListener := newTestListener()
			grpcListener := newTestListener()
			failed, sibling := httpListener, grpcListener
			if failedProtocol == "gRPC" {
				failed, sibling = grpcListener, httpListener
			}
			failed.failure = failure

			if err := serve(t.Context(), &http.Server{}, httpListener, grpc.NewServer(), grpcListener); !errors.Is(err, failure) {
				t.Fatalf("serve returned %v; want listener error", err)
			}
			select {
			case <-sibling.closed:
			default:
				t.Fatal("other listener still open after server failure")
			}
		})
	}
}

type testListener struct {
	closed  chan struct{}
	once    sync.Once
	failure error
}

func newTestListener() *testListener {
	return &testListener{closed: make(chan struct{})}
}

func (l *testListener) Accept() (net.Conn, error) {
	if l.failure != nil {
		return nil, l.failure
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *testListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *testListener) Addr() net.Addr { return &net.TCPAddr{} }
