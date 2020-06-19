package dependency

import "context"

// FakeDAGManager is a mock implementation for testing.
type FakeDAGManager struct {
}

// NewFakeDAGManager returns a FakeDAGManager for testing.
func NewFakeDAGManager() DAGManager {
	return &FakeDAGManager{}
}

// Start implements the DAGManager.Start method.
func (dm *FakeDAGManager) Start(ctx context.Context) {
}

// AddDAG implements the DAGManager.AddDAG method.
func (dm *FakeDAGManager) AddDAG(appKey string, dag *DAG) {
}
