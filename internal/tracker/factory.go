package tracker

import (
	"fmt"

	"github.com/shivamstaq/github-symphony/internal/config"
)

// TrackerFactory creates a Tracker from config.
// Each tracker kind registers its constructor here.
type TrackerFactory func(cfg *config.SymphonyConfig) (Tracker, error)

var factories = map[string]TrackerFactory{}

// Register adds a tracker factory for a given kind.
func Register(kind string, factory TrackerFactory) {
	factories[kind] = factory
}

// NewTracker creates a Tracker based on the config's tracker.kind.
func NewTracker(cfg *config.SymphonyConfig) (Tracker, error) {
	kind := cfg.Tracker.Kind
	factory, ok := factories[kind]
	if !ok {
		return nil, fmt.Errorf("unknown tracker kind %q — supported: github, linear", kind)
	}
	return factory(cfg)
}
