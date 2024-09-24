package store

import (
	"errors"

	buildpb "github.com/michaeledgar/bes-server/genproto/google/devtools/build/v1"
)

var (
	ErrStreamAlreadyExists   error = errors.New("stream already exists")
	ErrStreamAlreadyFinished error = errors.New("stream already finished")
	ErrStreamNotFound        error = errors.New("stream not found")
	ErrNotLifecycleEvent     error = errors.New("not a lifecycle event")
)

// BuildEvent represents a single build event
type BuildEvent struct {
	Content *buildpb.OrderedBuildEvent
}

type BuildEventStore interface {
	StoreLifecycleEvent(be *buildpb.OrderedBuildEvent) error
	StoreBuildToolEvent(be *buildpb.OrderedBuildEvent) error
	GetAllInvocationEvents(buildID string) ([]BuildEvent, error)
}
