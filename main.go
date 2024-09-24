package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/michaeledgar/bes-server/server"
	eventstore "github.com/michaeledgar/bes-server/server/store"

	_ "github.com/michaeledgar/bes-server/genproto/buildeventstream"
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

	besStore := eventstore.NewMemoryBuildEventStore()

	// Start gRPC server in separate goroutine.
	startGRPC, stopGRPC, err := server.MakeGRPCServer(besStore, server.GRPCConfig{
		Port:      *grpcPort,
		FailSetup: die,
	})
	if err != nil {
		die("makeGRPCServer", err)
	}
	go startGRPC()

	// Start HTTP server in separate goroutine.
	startHTTP, stopHTTP, err := server.MakeHTTPServer(ctx, besStore, server.HttpConfig{
		Port:            *httpPort,
		ReadTimeout:     *httpReadTimeout,
		WriteTimeout:    *httpWriteTimeout,
		IdleTimeout:     *httpIdleTimeout,
		ShutdownTimeout: *httpShutdownTimeout,
		FailSetup:       die,
	})
	if err != nil {
		die("makeHTTPServer", err)
	}
	go startHTTP()

	// Wait for interrupt signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	// Graceful shutdown.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		stopGRPC()
		fmt.Printf("gRPC Server stopped.\n")
	}()
	go func() {
		defer wg.Done()
		stopHTTP()
		fmt.Printf("HTTP Server stopped.\n")
	}()
	wg.Wait()
	log.Println("All servers stopped. Exiting.")
}

func die(s string, e error) {
	fmt.Printf("Failed startup: %v: %v\n", s, e)
	os.Exit(1)
}
