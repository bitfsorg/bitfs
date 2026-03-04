package engine

import libengine "github.com/bitfsorg/libbitfs-go/engine"

// Engine is a thin alias to the shared libbitfs-go engine implementation.
type Engine = libengine.Engine

// New creates a new engine instance.
func New(dataDir, password string) *Engine {
	return libengine.New(dataDir, password)
}
