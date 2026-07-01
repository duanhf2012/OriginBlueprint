// Package golang implements the Go runtime for Origin blueprint graphs.
//
// The runtime keeps compiled graph structure immutable and stores per-create
// mutable state in GraphInstance, so many server objects can share one compiled
// execution tree while keeping their own variables and timers.
package golang
