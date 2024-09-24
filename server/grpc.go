package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"

	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
	eventstore "github.com/michaeledgar/bes-server/server/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GRPCConfig struct {
	Port int

	FailSetup func(string, error)
}

type grpcServerFunc func()

// MakeGRPCServer creates the gRPC server with the given configuration and returns it, ready to be started.
func MakeGRPCServer(besStore eventstore.BuildEventStore, config GRPCConfig) (grpcServerFunc, grpcServerFunc, error) {
	besSrv := NewPublishBuildEventServer(besStore)

	srv := grpc.NewServer()
	buildpb.RegisterPublishBuildEventServer(srv, besSrv)
	reflection.Register(srv)

	fmt.Printf("Starting gRPC server on port: %d\n", config.Port)
	l, err := net.Listen("tcp", "localhost:"+strconv.Itoa(config.Port))
	if err != nil {
		return nil, nil, fmt.Errorf("net.Listen: %v", err)
	}

	return func() {
			err = srv.Serve(l)
			if err != nil {
				config.FailSetup("srv.Serve()", err)
			}
		}, func() {
			srv.GracefulStop()
		}, nil
}

// PublishBuildEventServer implements the PublishBuildEvent service.
type PublishBuildEventServer struct {
	buildpb.UnimplementedPublishBuildEventServer
	store eventstore.BuildEventStore
}

// NewPublishBuildEventServer creates a new PublishBuildEventServer.
func NewPublishBuildEventServer(store eventstore.BuildEventStore) *PublishBuildEventServer {
	return &PublishBuildEventServer{store: store}
}

// PublishLifecycleEvent implements the PublishLifecycleEvent RPC.
func (s *PublishBuildEventServer) PublishLifecycleEvent(ctx context.Context, req *buildpb.PublishLifecycleEventRequest) (*emptypb.Empty, error) {
	buildID := req.BuildEvent.StreamId.BuildId
	if err := s.store.StoreLifecycleEvent(req.BuildEvent); err != nil {
		if err == eventstore.ErrNotLifecycleEvent {
			return nil, status.Errorf(codes.InvalidArgument, "failed to store lifecycle event: %v", err)
		} else if err == eventstore.ErrStreamNotFound {
			return nil, status.Errorf(codes.NotFound, "failed to store lifecycle event: %v", err)
		}
		return nil, err
	}
	log.Printf("Received lifecycle event for build ID: %s", buildID)
	return &emptypb.Empty{}, nil
}

// PublishBuildToolEventStream implements the PublishBuildToolEventStream RPC.
func (s *PublishBuildEventServer) PublishBuildToolEventStream(stream buildpb.PublishBuildEvent_PublishBuildToolEventStreamServer) error {
	defer func() {
		log.Printf("Exiting PublishBuildToolEventStream()")
	}()
	for {
		req, err := stream.Recv()
		if err != nil {
			return status.Errorf(codes.Internal, "failed to receive event: %v", err)
		}
		obe := req.OrderedBuildEvent
		if err = s.store.StoreBuildToolEvent(obe); err != nil {
			if err == eventstore.ErrStreamNotFound {
				return status.Errorf(codes.NotFound, "failed to store event: %v", err)
			}
			return status.Errorf(codes.Internal, "failed to store event: %v", err)
		}

		log.Printf("Received build tool event for build ID: %s", obe.StreamId.BuildId)

		if err := stream.Send(&buildpb.PublishBuildToolEventStreamResponse{
			StreamId:       obe.StreamId,
			SequenceNumber: obe.SequenceNumber,
		}); err != nil {
			return status.Errorf(codes.Internal, "failed to send response: %v", err)
		}

		if obe.Event.GetComponentStreamFinished() != nil && obe.Event.GetComponentStreamFinished().Type == buildpb.BuildEvent_BuildComponentStreamFinished_FINISHED {
			return nil
		}
	}
}

// GetBuildEvents retrieves all events for a given invocation ID.
func (s *PublishBuildEventServer) GetBuildEvents(invocationID string) ([]eventstore.BuildEvent, error) {
	return s.store.GetAllInvocationEvents(invocationID)
}
