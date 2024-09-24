package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
	"github.com/michaeledgar/bes-server/server/store"
	testpb "github.com/michaeledgar/bes-server/testproto"
	"google.golang.org/protobuf/types/known/anypb"
)

func setupTestServer() (*httptest.Server, *store.MemoryBuildEventStore) {
	besStore := store.NewMemoryBuildEventStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/", helloHandler)
	mux.HandleFunc("/builds/", buildHandler(besStore))

	server := httptest.NewServer(logMiddleware(mux))
	return server, besStore
}

func TestHelloHandler(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK; got %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!'; got %s", string(body))
	}
}

func TestHelloHandler_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status Not Found; got %v", resp.Status)
	}
}

func TestBuildHandler_MissingInvocationID(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/builds/")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request; got %v", resp.Status)
	}
}

func TestBuildHandler_InvalidInvocationID(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/builds/invalid-uuid")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request; got %v", resp.Status)
	}
}

func TestBuildHandler_NonexistentInvocationID(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	invocationID := uuid.New().String()
	resp, err := http.Get(server.URL + "/builds/" + invocationID)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status Not Found; got %v", resp.Status)
	}
}

func TestBuildHandler_ValidInvocation(t *testing.T) {
	server, besStore := setupTestServer()
	defer server.Close()

	// Create a test build event
	invocationID := uuid.New().String()
	buildID := "test-build-id"
	testEvent := &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:      buildID,
			InvocationId: invocationID,
		},
		SequenceNumber: 1,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_BazelEvent{
				BazelEvent: &anypb.Any{},
			},
		},
	}
	progressEvent1 := &testpb.TestProgressEvent{
		Stdout: "Test stdout",
		Stderr: "Test stderr",
	}
	testEvent.Event.GetBazelEvent().MarshalFrom(progressEvent1)

	// Store the test event
	err := besStore.StoreBuildToolEvent(testEvent)
	if err != nil {
		t.Fatalf("Failed to store test event: %v", err)
	}

	// Send request
	resp, err := http.Get(server.URL + "/builds/" + invocationID)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK; got %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expectedSubstrings := []string{
		fmt.Sprintf("Valid invocation ID: %s", invocationID),
		"Event:",
		"build_id:",
		"invocation_id:",
		"\"Test stdout\"",
		"\"Test stderr\"",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(string(body), substr) {
			t.Errorf("Expected response to contain '%s', but it didn't: %s", substr, body)
		}
	}
}

func TestLogMiddleware(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK; got %v", resp.Status)
	}

	// Note: We can't easily test the log output directly in this setup.
	// In a real-world scenario, you might want to use a custom logger
	// that you can inspect in your tests.
}

func TestMakeHTTPServer(t *testing.T) {
	besStore := store.NewMemoryBuildEventStore()
	config := HttpConfig{
		Port:            0, // Use port 0 to let the system assign an available port
		ReadTimeout:     10 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     30 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		FailSetup: func(s string, err error) {
			t.Fatalf("Setup failed: %s - %v", s, err)
		},
	}

	ctx := context.Background()
	start, stop, err := MakeHTTPServer(ctx, besStore, config)
	if err != nil {
		t.Fatalf("MakeHTTPServer failed: %v", err)
	}

	// Start the server in a goroutine
	go start()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop the server
	stop()

	// Note: We can't easily test the actual serving of requests here
	// without knowing the assigned port. In a real-world scenario,
	// you might want to modify MakeHTTPServer to return the assigned
	// port, so you can test against it.
}
