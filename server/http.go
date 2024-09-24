package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
	"github.com/michaeledgar/bes-server/server/batchproto"
	eventstore "github.com/michaeledgar/bes-server/server/store"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
)

// HttpConfig is a plain data object containing all configurable settings in the HTTP Server.
type HttpConfig struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	FailSetup       func(string, error)
}

// httpServerFunc is a callback for starting or stopping an HTTP server.
type httpServerFunc func()

func MakeHTTPServer(ctx context.Context, besStore eventstore.BuildEventStore, config HttpConfig) (httpServerFunc, httpServerFunc, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", helloHandler)
	mux.HandleFunc("/builds/", buildHandler(besStore))
	httpServer := &http.Server{
		Handler:      logMiddleware(mux),
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	fmt.Printf("Starting HTTP server on port: %d\n", config.Port)
	httpL, err := net.Listen("tcp", "localhost:"+strconv.Itoa(config.Port))
	if err != nil {
		return nil, nil, fmt.Errorf("net.Listen: %v", err)
	}
	return func() {
			err = httpServer.Serve(httpL)
			if err != nil && err != http.ErrServerClosed {
				config.FailSetup("httpServer.Serve()", err)
			}
		}, func() {
			shutdownCtx, cancel := context.WithTimeout(ctx, config.ShutdownTimeout)
			defer cancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("HTTP server shutdown error: %v", err)
			}
			fmt.Printf("HTTP Server stopped.\n")
		}, nil
}

// loggableResponseWriter is an http.ResponseWriter that counts bytes written and stores returned status.
type loggableResponseWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (rw *loggableResponseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *loggableResponseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.length += n
	return n, err
}

// logMiddleware wraps an http.Handler with logging.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &loggableResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		log.Printf("%s %s - Status: %d, Bytes: %d, Duration: %s",
			r.Method, r.RequestURI, rw.status, rw.length, duration)
	})
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
func buildHandler(grpcSrv eventstore.BuildEventStore) http.HandlerFunc {
	marshal := prototext.MarshalOptions{
		Multiline: true,
	}
	jsonMarshal := protojson.MarshalOptions{Multiline: true}
	return func(w http.ResponseWriter, r *http.Request) {
		invocationID := strings.TrimPrefix(r.URL.Path, "/builds/")
		if invocationID == "" {
			http.Error(w, "Missing invocation ID", http.StatusBadRequest)
			return
		}
		format := "html"
		if strings.HasSuffix(invocationID, ".json") {
			format = "json"
			invocationID = strings.TrimSuffix(invocationID, ".json")
			w.Header().Add("Content-Type", "application/json")
		}

		_, err := uuid.Parse(invocationID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			mustWriteHTTP(w, "Invalid invocation ID", invocationID)
			return
		}

		evts, err := grpcSrv.GetAllInvocationEvents(invocationID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			mustWriteHTTP(w, fmt.Sprintf("invocation ID not found: %s", invocationID), invocationID)
			return
		}
		result := &batchproto.BuildEventBatch{
			Event: make([]*build.OrderedBuildEvent, 0, len(evts)),
		}
		for _, evt := range evts {
			result.Event = append(result.Event, evt.Content)
		}
		w.WriteHeader(http.StatusOK)
		switch format {
		case "html":
			if !mustWriteHTTP(w, fmt.Sprintf("<b>Valid invocation ID: %s</b><br/>", invocationID), invocationID) {
				return
			}
			for _, evt := range evts {
				if !mustWriteHTTP(w, "<b>Event:</b><br/><pre>", invocationID) {
					return
				}
				text, err := marshal.Marshal(evt.Content)
				if err != nil {
					log.Fatal(err)
				} else if !mustWriteHTTP(w, string(text), invocationID) {
					return
				}
			}
			if !mustWriteHTTP(w, "</pre><br />", invocationID) {
				return
			}
			break
		case "json":
			allJson, err := jsonMarshal.Marshal(result)
			if err != nil {
				log.Fatal(err)
			} else {
				if !mustWriteHTTP(w, string(allJson), invocationID) {
					return
				}
			}
			break
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
