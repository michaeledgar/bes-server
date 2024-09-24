package server

import (
	"context"
	"net"
	"testing"
	"time"

	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
	"github.com/michaeledgar/bes-server/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const bufSize = 1024 * 1024

// MockBuildEventStore is a mock implementation of the BuildEventStore interface
type MockBuildEventStore struct {
	mock.Mock
}

func (m *MockBuildEventStore) StoreLifecycleEvent(be *buildpb.OrderedBuildEvent) error {
	args := m.Called(be)
	return args.Error(0)
}

func (m *MockBuildEventStore) StoreBuildToolEvent(be *buildpb.OrderedBuildEvent) error {
	args := m.Called(be)
	return args.Error(0)
}

func (m *MockBuildEventStore) GetAllInvocationEvents(buildID string) ([]store.BuildEvent, error) {
	args := m.Called(buildID)
	return args.Get(0).([]store.BuildEvent), args.Error(1)
}

// protoMatcher is a custom matcher for comparing protobuf messages
func protoMatcher(expected proto.Message) interface{} {
	return mock.MatchedBy(func(actual proto.Message) bool {
		return proto.Equal(expected, actual)
	})
}

func setupGRPCServer(mockStore *MockBuildEventStore) (buildpb.PublishBuildEventClient, func()) {
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	buildpb.RegisterPublishBuildEventServer(srv, NewPublishBuildEventServer(mockStore))
	go func() {
		if err := srv.Serve(lis); err != nil {
			panic(err)
		}
	}()
	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(bufDialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}

	client := buildpb.NewPublishBuildEventClient(conn)

	return client, func() {
		conn.Close()
		srv.GracefulStop()
	}
}

func TestPublishLifecycleEvent(t *testing.T) {
	mockStore := new(MockBuildEventStore)
	client, cleanup := setupGRPCServer(mockStore)
	defer cleanup()

	testCases := []struct {
		name          string
		buildEvent    *buildpb.OrderedBuildEvent
		storeError    error
		expectedError error
	}{
		{
			name: "Successful lifecycle event",
			buildEvent: &buildpb.OrderedBuildEvent{
				StreamId:       &buildpb.StreamId{BuildId: "build-1"},
				SequenceNumber: 1,
				Event: &buildpb.BuildEvent{
					Event: &buildpb.BuildEvent_BuildEnqueued_{
						BuildEnqueued: &buildpb.BuildEvent_BuildEnqueued{
							Details: nil,
						},
					},
				},
			},
			storeError:    nil,
			expectedError: nil,
		},
		{
			name: "Not a lifecycle event",
			buildEvent: &buildpb.OrderedBuildEvent{
				StreamId:       &buildpb.StreamId{BuildId: "build-2"},
				SequenceNumber: 1,
				Event: &buildpb.BuildEvent{
					Event: &buildpb.BuildEvent_BazelEvent{
						BazelEvent: &anypb.Any{},
					},
				},
			},
			storeError:    store.ErrNotLifecycleEvent,
			expectedError: status.Error(codes.InvalidArgument, "failed to store lifecycle event: not a lifecycle event"),
		},
		{
			name: "Stream not found",
			buildEvent: &buildpb.OrderedBuildEvent{
				StreamId:       &buildpb.StreamId{BuildId: "build-3"},
				SequenceNumber: 2,
				Event: &buildpb.BuildEvent{
					Event: &buildpb.BuildEvent_BuildFinished_{
						BuildFinished: &buildpb.BuildEvent_BuildFinished{
							Details: nil,
						},
					},
				},
			},
			storeError:    store.ErrStreamNotFound,
			expectedError: status.Error(codes.NotFound, "failed to store lifecycle event: stream not found"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore.On("StoreLifecycleEvent", protoMatcher(tc.buildEvent)).Return(tc.storeError).Once()

			_, err := client.PublishLifecycleEvent(context.Background(), &buildpb.PublishLifecycleEventRequest{
				BuildEvent: tc.buildEvent,
			})

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestPublishBuildToolEventStream(t *testing.T) {
	mockStore := new(MockBuildEventStore)
	client, cleanup := setupGRPCServer(mockStore)
	defer cleanup()

	testCases := []struct {
		name          string
		events        []*buildpb.OrderedBuildEvent
		storeErrors   []error
		expectedError error
	}{
		{
			name: "Successful stream",
			events: []*buildpb.OrderedBuildEvent{
				{
					StreamId:       &buildpb.StreamId{BuildId: "build-1", InvocationId: "inv-1"},
					SequenceNumber: 1,
					Event: &buildpb.BuildEvent{
						Event: &buildpb.BuildEvent_BazelEvent{
							BazelEvent: &anypb.Any{},
						},
					},
				},
				{
					StreamId:       &buildpb.StreamId{BuildId: "build-1", InvocationId: "inv-1"},
					SequenceNumber: 2,
					Event: &buildpb.BuildEvent{
						Event: &buildpb.BuildEvent_ComponentStreamFinished{
							ComponentStreamFinished: &buildpb.BuildEvent_BuildComponentStreamFinished{
								Type: buildpb.BuildEvent_BuildComponentStreamFinished_FINISHED,
							},
						},
					},
				},
			},
			storeErrors:   []error{nil, nil},
			expectedError: nil,
		},
		{
			name: "Stream not found",
			events: []*buildpb.OrderedBuildEvent{
				{
					StreamId:       &buildpb.StreamId{BuildId: "build-2", InvocationId: "inv-2"},
					SequenceNumber: 1,
					Event: &buildpb.BuildEvent{
						Event: &buildpb.BuildEvent_BazelEvent{
							BazelEvent: &anypb.Any{},
						},
					},
				},
			},
			storeErrors:   []error{store.ErrStreamNotFound},
			expectedError: status.Error(codes.NotFound, "failed to store event: stream not found"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			stream, err := client.PublishBuildToolEventStream(ctx)
			assert.NoError(t, err)

			for i, event := range tc.events {
				mockStore.On("StoreBuildToolEvent", protoMatcher(event)).Return(tc.storeErrors[i]).Once()

				err := stream.Send(&buildpb.PublishBuildToolEventStreamRequest{OrderedBuildEvent: event})
				assert.NoError(t, err)

				resp, err := stream.Recv()
				if tc.storeErrors[i] != nil {
					assert.Error(t, err)
					assert.Equal(t, tc.expectedError.Error(), err.Error())
					break
				} else {
					assert.NoError(t, err)
					assert.True(t, proto.Equal(event.StreamId, resp.StreamId), "StreamID is equal")
					assert.Equal(t, event.SequenceNumber, resp.SequenceNumber)
				}
			}

			err = stream.CloseSend()
			assert.NoError(t, err)

			mockStore.AssertExpectations(t)
		})
	}
}

func TestGetBuildEvents(t *testing.T) {
	// Note: GetBuildEvents is not part of the gRPC service definition,
	// so we'll test it directly on the server instance

	mockStore := new(MockBuildEventStore)
	server := NewPublishBuildEventServer(mockStore)

	testCases := []struct {
		name           string
		invocationID   string
		mockEvents     []store.BuildEvent
		mockError      error
		expectedEvents []store.BuildEvent
		expectedError  error
	}{
		{
			name:         "Successful retrieval",
			invocationID: "inv-1",
			mockEvents: []store.BuildEvent{
				{Content: &buildpb.OrderedBuildEvent{SequenceNumber: 1}},
				{Content: &buildpb.OrderedBuildEvent{SequenceNumber: 2}},
			},
			mockError:      nil,
			expectedEvents: []store.BuildEvent{{Content: &buildpb.OrderedBuildEvent{SequenceNumber: 1}}, {Content: &buildpb.OrderedBuildEvent{SequenceNumber: 2}}},
			expectedError:  nil,
		},
		{
			name:           "Stream not found",
			invocationID:   "inv-2",
			mockEvents:     nil,
			mockError:      store.ErrStreamNotFound,
			expectedEvents: nil,
			expectedError:  store.ErrStreamNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore.On("GetAllInvocationEvents", tc.invocationID).Return(tc.mockEvents, tc.mockError).Once()

			events, err := server.GetBuildEvents(tc.invocationID)

			assert.Equal(t, tc.expectedEvents, events)
			assert.Equal(t, tc.expectedError, err)

			mockStore.AssertExpectations(t)
		})
	}
}
