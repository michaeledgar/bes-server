package store

import (
	"reflect"
	"testing"

	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
	testpb "github.com/michaeledgar/bes-server/testproto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	fakeBuildID       = "fake-buildid"
	fakeInvocationID1 = "fake-invocationid-1"
)

var (
	buildEnqueuedEvent_Valid = &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:   fakeBuildID,
			Component: buildpb.StreamId_CONTROLLER,
		},
		SequenceNumber: 1,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_BuildEnqueued_{
				BuildEnqueued: &buildpb.BuildEvent_BuildEnqueued{
					Details: nil,
				},
			},
		},
	}
	buildFinishedEvent_Valid = &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:   fakeBuildID,
			Component: buildpb.StreamId_CONTROLLER,
		},
		SequenceNumber: 2,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_BuildFinished_{
				BuildFinished: &buildpb.BuildEvent_BuildFinished{
					Details: nil,
				},
			},
		},
	}
	invocationAttemptStarted1_Valid = &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:      fakeBuildID,
			InvocationId: fakeInvocationID1,
			Component:    buildpb.StreamId_WORKER,
		},
		SequenceNumber: 1,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_InvocationAttemptStarted_{
				InvocationAttemptStarted: &buildpb.BuildEvent_InvocationAttemptStarted{
					AttemptNumber: 1,
					Details:       nil,
				},
			},
		},
	}
	invocationAttemptFinished1_Valid = &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:      fakeBuildID,
			InvocationId: fakeInvocationID1,
			Component:    buildpb.StreamId_WORKER,
		},
		SequenceNumber: 2,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_InvocationAttemptFinished_{
				InvocationAttemptFinished: &buildpb.BuildEvent_InvocationAttemptFinished{
					Details: nil,
				},
			},
		},
	}
	progressEvent1_Wrapped_Valid = &buildpb.OrderedBuildEvent{
		StreamId: &buildpb.StreamId{
			BuildId:      fakeBuildID,
			InvocationId: fakeInvocationID1,
			Component:    buildpb.StreamId_TOOL,
		},
		SequenceNumber: 1,
		Event: &buildpb.BuildEvent{
			Event: &buildpb.BuildEvent_BazelEvent{
				BazelEvent: &anypb.Any{},
			},
		},
	}
	progressEvent1 = &testpb.TestProgressEvent{
		Stdout: "my-stdout",
		Stderr: "my-stderr",
	}
)

func init() {
	progressEvent1_Wrapped_Valid.Event.GetBazelEvent().MarshalFrom(progressEvent1)
}

func TestStoreAllEvents_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
	if got := s.StoreBuildToolEvent(progressEvent1_Wrapped_Valid); got != nil {
		t.Errorf("StoreBuildToolEvent(%v) = %v, want nil", progressEvent1_Wrapped_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptFinished1_Valid, got)
	}
	if got := s.StoreLifecycleEvent(buildFinishedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
}

func TestPublishLifecycleEvent_BuildEnqueued_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
}

func TestPublishLifecycleEvent_BuildEnqueued_AlreadyExists(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got, want := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid), ErrStreamAlreadyExists; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", buildEnqueuedEvent_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_BuildFinished_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(buildFinishedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
}

func TestPublishLifecycleEvent_BuildFinished_BuildNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got, want := s.StoreLifecycleEvent(buildFinishedEvent_Valid), ErrStreamNotFound; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", buildFinishedEvent_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptStarted_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptStarted_BuildNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got, want := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid), ErrStreamNotFound; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", invocationAttemptStarted1_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptStarted_StreamAlreadyExists(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
	if got, want := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid), ErrStreamAlreadyExists; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", invocationAttemptStarted1_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptFinished_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptFinished1_Valid, got)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptFinished_BuildNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got, want := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid), ErrStreamNotFound; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", invocationAttemptFinished1_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptFinished_InvocationNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got, want := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid), ErrStreamNotFound; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", invocationAttemptFinished1_Valid, got, want)
	}
}

func TestPublishLifecycleEvent_InvocationAttemptFinished_InvocationAlreadyFinished(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptFinished1_Valid, got)
	}
	if got, want := s.StoreLifecycleEvent(invocationAttemptFinished1_Valid), ErrStreamAlreadyFinished; got != want {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want %v", invocationAttemptFinished1_Valid, got, want)
	}
}

func TestStoreBuildToolEvent_NoLifecycleEvents_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreBuildToolEvent(progressEvent1_Wrapped_Valid); got != nil {
		t.Errorf("StoreBuildToolEvent(%v) = %v, want nil", progressEvent1_Wrapped_Valid, got)
	}
}

func TestStoreBuildToolEvent_WithLifecycleEvents_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got := s.StoreLifecycleEvent(invocationAttemptStarted1_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", invocationAttemptStarted1_Valid, got)
	}
	if got := s.StoreBuildToolEvent(progressEvent1_Wrapped_Valid); got != nil {
		t.Errorf("StoreBuildToolEvent(%v) = %v, want nil", progressEvent1_Wrapped_Valid, got)
	}
}

func TestStoreBuildToolEvent_InvocationNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	if got := s.StoreLifecycleEvent(buildEnqueuedEvent_Valid); got != nil {
		t.Errorf("StoreLifecycleEvent(%v) = %v, want nil", buildEnqueuedEvent_Valid, got)
	}
	if got, want := s.StoreBuildToolEvent(progressEvent1_Wrapped_Valid), ErrStreamNotFound; got != want {
		t.Errorf("StoreBuildToolEvent(%v) = %v, want %v", progressEvent1_Wrapped_Valid, got, want)
	}
}

func TestGetAllInvocationEvents_InvocationNotFound(t *testing.T) {
	s := NewMemoryBuildEventStore()

	evts, err := s.GetAllInvocationEvents(fakeInvocationID1)
	if evts != nil {
		t.Errorf("GetAllInvocationEvents(%v) = %v, want %v", fakeInvocationID1, evts, nil)
	}
	if got, want := err, ErrStreamNotFound; got != want {
		t.Errorf("GetAllInvocationEvents(%v) = %v, want %v", fakeInvocationID1, got, want)
	}
}

func TestGetAllInvocationEvents_Good(t *testing.T) {
	s := NewMemoryBuildEventStore()

	s.StoreBuildToolEvent(progressEvent1_Wrapped_Valid)
	evts, err := s.GetAllInvocationEvents(fakeInvocationID1)
	if got, want := evts, []BuildEvent{{progressEvent1_Wrapped_Valid}}; !reflect.DeepEqual(got, want) {
		t.Errorf("GetAllInvocationEvents(%v) = %v, want %v", fakeInvocationID1, got, want)
	}
	if err != nil {
		t.Errorf("GetAllInvocationEvents(%v) error = %v, want %v", fakeInvocationID1, err, nil)
	}
}
