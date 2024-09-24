package store

import (
	"sync"

	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
)

// MemoryBuildEventStore stores build events in memory
type MemoryBuildEventStore struct {
	mu sync.RWMutex
	// events is keyed by Invocation ID.
	events map[string][]BuildEvent
	// lifecycleEvents is keyed by Build ID.
	lifecycleEvents map[string][]*buildpb.OrderedBuildEvent
}

// NewMemoryBuildEventStore creates a new MemoryBuildEventStore
func NewMemoryBuildEventStore() *MemoryBuildEventStore {
	return &MemoryBuildEventStore{
		events:          make(map[string][]BuildEvent),
		lifecycleEvents: make(map[string][]*buildpb.OrderedBuildEvent),
	}
}

func (s *MemoryBuildEventStore) StoreLifecycleEvent(obe *buildpb.OrderedBuildEvent) error {
	be := obe.Event
	if be.GetInvocationAttemptStarted() == nil && be.GetInvocationAttemptFinished() == nil && be.GetBuildEnqueued() == nil && be.GetBuildFinished() == nil {
		return ErrNotLifecycleEvent
	}
	buildID := obe.StreamId.BuildId
	invocationID := obe.StreamId.InvocationId
	s.mu.Lock()
	defer s.mu.Unlock()
	// If we're creating the build ID, verify it doesn't exist yet. Otherwise, verify it *does* exist.
	if be.GetBuildEnqueued() != nil {
		if _, ok := s.lifecycleEvents[buildID]; ok {
			return ErrStreamAlreadyExists
		}
	} else if be.GetBuildFinished() != nil || be.GetInvocationAttemptStarted() != nil || be.GetInvocationAttemptFinished() != nil {
		if _, ok := s.lifecycleEvents[buildID]; !ok {
			return ErrStreamNotFound
		}
	}
	if be.GetInvocationAttemptStarted() != nil && s.checkInvocationExists(buildID, invocationID) {
		return ErrStreamAlreadyExists
	}
	if be.GetInvocationAttemptFinished() != nil && !s.checkInvocationExists(buildID, invocationID) {
		return ErrStreamNotFound
	}
	if be.GetInvocationAttemptFinished() != nil && s.checkInvocationFinished(buildID, invocationID) {
		return ErrStreamAlreadyFinished
	}
	s.lifecycleEvents[buildID] = append(s.lifecycleEvents[buildID], obe)
	return nil
}

func (s *MemoryBuildEventStore) StoreBuildToolEvent(be *buildpb.OrderedBuildEvent) error {
	buildID := be.StreamId.BuildId
	invocationID := be.StreamId.InvocationId
	s.mu.Lock()
	defer s.mu.Unlock()

	// If this build ID has lifecycle events then the invocation must exist.
	if _, ok := s.lifecycleEvents[buildID]; ok {
		if !s.checkInvocationExists(buildID, invocationID) {
			return ErrStreamNotFound
		}
	}

	event := BuildEvent{
		Content: be,
	}
	s.events[invocationID] = append(s.events[invocationID], event)
	return nil
}

func (s *MemoryBuildEventStore) GetAllInvocationEvents(invocationID string) ([]BuildEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, ok := s.events[invocationID]
	if !ok {
		return nil, ErrStreamNotFound
	}
	return events, nil
}

func (s *MemoryBuildEventStore) checkInvocationExists(buildID, invocationID string) bool {
	if levts, ok := s.lifecycleEvents[buildID]; ok {
		for _, levt := range levts {
			if levt.Event.GetInvocationAttemptStarted() != nil && levt.StreamId.InvocationId == invocationID {
				return true
			}
		}
	}
	return false
}

func (s *MemoryBuildEventStore) checkInvocationFinished(buildID, invocationID string) bool {
	if levts, ok := s.lifecycleEvents[buildID]; ok {
		for _, levt := range levts {
			if levt.Event.GetInvocationAttemptFinished() != nil && levt.StreamId.InvocationId == invocationID {
				return true
			}
		}
	}
	return false
}
