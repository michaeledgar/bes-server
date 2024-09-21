package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/google/uuid"
	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
)

var (
	grpcPort = flag.Int("grpc_port", 3000, "TCP Port to use to serve gRPC server")
	httpPort = flag.Int("http_port", 3001, "TCP Port to use to serve HTTP server")

	httpReadTimeout     = flag.Duration("read-timeout", 5*time.Second, "HTTP server read timeout")
	httpWriteTimeout    = flag.Duration("write-timeout", 10*time.Second, "HTTP server write timeout")
	httpIdleTimeout     = flag.Duration("idle-timeout", 120*time.Second, "HTTP server idle timeout")
	httpShutdownTimeout = flag.Duration("shutdown-timeout", 15*time.Second, "Graceful shutdown timeout")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	// Start gRPC server in separate goroutine.
	besStore := NewBuildEventStore()
	besSrv := NewPublishBuildEventServer(besStore)

	srv := grpc.NewServer()
	buildpb.RegisterPublishBuildEventServer(srv, besSrv)
	reflection.Register(srv)

	fmt.Printf("Starting gRPC server on port: %d\n", *grpcPort)
	l, err := net.Listen("tcp", "localhost:"+strconv.Itoa(*grpcPort))
	if err != nil {
		die("net.Listen", err)
	}

	go func() {
		err = srv.Serve(l)
		if err != nil {
			die("srv.Serve()", err)
		}
	}()

	// Start HTTP server in separate goroutine.
	mux := http.NewServeMux()
	mux.HandleFunc("/", helloHandler)
	mux.HandleFunc("/builds/", buildHandler(besSrv))
	httpServer := &http.Server{
		Handler:      logMiddleware(mux),
		ReadTimeout:  *httpReadTimeout,
		WriteTimeout: *httpWriteTimeout,
		IdleTimeout:  *httpIdleTimeout,
	}

	fmt.Printf("Starting HTTP server on port: %d\n", *httpPort)
	httpL, err := net.Listen("tcp", "localhost:"+strconv.Itoa(*httpPort))
	if err != nil {
		die("net.Listen", err)
	}

	go func() {
		err = httpServer.Serve(httpL)
		if err != nil && err != http.ErrServerClosed {
			die("httpServer.Serve()", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	// Graceful shutdown
	var wg sync.WaitGroup
	wg.Add(2)

	// After interruption, tear down servers.
	go func() {
		defer wg.Done()
		srv.GracefulStop()
		fmt.Printf("gRPC Server stopped.\n")
	}()

	go func() {
		defer wg.Done()
		shutdownCtx, cancel := context.WithTimeout(ctx, *httpShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		fmt.Printf("HTTP Server stopped.\n")
	}()

	wg.Wait()
	log.Println("All servers stopped. Exiting.")
}

type responseWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.length += n
	return n, err
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		log.Printf("%s %s - Status: %d, Bytes: %d, Duration: %s",
			r.Method, r.RequestURI, rw.status, rw.length, duration)
	})
}

func die(s string, e error) {
	fmt.Printf("Failed startup: %v: %v\n", s, e)
	os.Exit(1)
}

// Handler for /
func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Hello, World!")); err != nil {
		log.Printf("Failed to write Hello, World! response: %v", err)
		return
	}
}

// Handler for /builds/*
func buildHandler(grpcSrv *PublishBuildEventServer) http.HandlerFunc {
	marshal := prototext.MarshalOptions{
		Multiline: true,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		buildID := strings.TrimPrefix(r.URL.Path, "/builds/")
		if buildID == "" {
			http.Error(w, "Missing build ID", http.StatusBadRequest)
			return
		}

		_, err := uuid.Parse(buildID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			mustWriteHTTP(w, "Invalid build ID", buildID)
			return
		}

		evts, err := grpcSrv.GetBuildEvents(buildID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			mustWriteHTTP(w, fmt.Sprintf("Build ID not found: %s", buildID), buildID)
			return
		}
		w.WriteHeader(http.StatusOK)
		if !mustWriteHTTP(w, "<b>Valid build ID: %s</b><br/>", buildID) {
			return
		}
		for _, evt := range evts {
			if !mustWriteHTTP(w, "<b>Event:</b><br/><pre>", buildID) {
				return
			}
			text, err := marshal.Marshal(evt.Content)
			if err != nil {
				log.Fatal(err)
			}
			if !mustWriteHTTP(w, string(text), buildID) {
				return
			}
		}
		if !mustWriteHTTP(w, "</pre><br />", buildID) {
			return
		}
	}
}

// mustWriteHTTP tries to write the given string to the HTTP response, and if it fails, logs the build ID and returns false.
func mustWriteHTTP(w http.ResponseWriter, s, buildID string) bool {
	if _, err := w.Write([]byte(s)); err != nil {
		log.Printf("Failed to write to HTTP response %s: %v", buildID, err)
		return false
	}
	return true
}

// BuildEvent represents a single build event
type BuildEvent struct {
	Type    string
	Content proto.Message
}

// BuildEventStore stores build events in memory
type BuildEventStore struct {
	mu     sync.RWMutex
	events map[string][]BuildEvent
}

// PublishBuildEventServer implements the PublishBuildEvent service
type PublishBuildEventServer struct {
	buildpb.UnimplementedPublishBuildEventServer
	store *BuildEventStore
}

// NewBuildEventStore creates a new BuildEventStore
func NewBuildEventStore() *BuildEventStore {
	return &BuildEventStore{
		events: make(map[string][]BuildEvent),
	}
}

// NewPublishBuildEventServer creates a new PublishBuildEventServer
func NewPublishBuildEventServer(store *BuildEventStore) *PublishBuildEventServer {
	return &PublishBuildEventServer{store: store}
}

// PublishLifecycleEvent implements the PublishLifecycleEvent RPC
func (s *PublishBuildEventServer) PublishLifecycleEvent(ctx context.Context, req *buildpb.PublishLifecycleEventRequest) (*emptypb.Empty, error) {
	buildID := req.BuildEvent.StreamId.BuildId

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	event := BuildEvent{
		Type:    "LifecycleEvent",
		Content: req.BuildEvent,
	}
	s.store.events[buildID] = append(s.store.events[buildID], event)

	log.Printf("Received lifecycle event for build ID: %s", buildID)
	return &emptypb.Empty{}, nil
}

// PublishBuildToolEventStream implements the PublishBuildToolEventStream RPC
func (s *PublishBuildEventServer) PublishBuildToolEventStream(stream buildpb.PublishBuildEvent_PublishBuildToolEventStreamServer) error {
	defer func() {
		log.Printf("Exiting PublishBuildToolEventStream()")
	}()
	for {
		req, err := stream.Recv()
		if err != nil {
			return status.Errorf(codes.Internal, "failed to receive event: %v", err)
		}

		buildID := req.OrderedBuildEvent.StreamId.BuildId

		s.store.mu.Lock()
		event := BuildEvent{
			Type:    "BuildToolEvent",
			Content: req.OrderedBuildEvent,
		}
		s.store.events[buildID] = append(s.store.events[buildID], event)
		s.store.mu.Unlock()

		log.Printf("Received build tool event for build ID: %s", buildID)

		if err := stream.Send(&buildpb.PublishBuildToolEventStreamResponse{
			StreamId:       req.OrderedBuildEvent.StreamId,
			SequenceNumber: req.OrderedBuildEvent.SequenceNumber,
		}); err != nil {
			return status.Errorf(codes.Internal, "failed to send response: %v", err)
		}

		if req.OrderedBuildEvent.Event.GetComponentStreamFinished() != nil && req.OrderedBuildEvent.Event.GetComponentStreamFinished().Type == buildpb.BuildEvent_BuildComponentStreamFinished_FINISHED {
			return nil
		}
	}
}

// GetBuildEvents retrieves all events for a given build ID
func (s *PublishBuildEventServer) GetBuildEvents(buildID string) ([]BuildEvent, error) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()

	events, ok := s.store.events[buildID]
	if !ok {
		return nil, fmt.Errorf("no events found for build ID: %s", buildID)
	}
	return events, nil
}
