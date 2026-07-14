package blueprint

import (
	"sync"
	"testing"
)

type manualExecutionDispatcher struct {
	mu       sync.Mutex
	tasks    []func()
	rejected bool
}

type testEntrance struct{ BaseExecNode }

func (n *testEntrance) GetName() string { return "TestEntrance" }
func (n *testEntrance) Exec() (int, error) {
	return 0, nil
}

type testRecorder struct {
	BaseExecNode
	mu     sync.Mutex
	values []PortInt
}

func (n *testRecorder) GetName() string { return "TestRecorder" }
func (n *testRecorder) Exec() (int, error) {
	value, ok := n.GetInPortInt(1)
	if ok {
		n.mu.Lock()
		n.values = append(n.values, value)
		n.mu.Unlock()
	}
	return -1, nil
}

func (n *testRecorder) snapshot() []PortInt {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]PortInt(nil), n.values...)
}

func (d *manualExecutionDispatcher) Submit(task func()) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rejected {
		return ErrExecutionRejected
	}
	d.tasks = append(d.tasks, task)
	return nil
}

func (d *manualExecutionDispatcher) runNext(t *testing.T) {
	t.Helper()
	d.mu.Lock()
	if len(d.tasks) == 0 {
		d.mu.Unlock()
		t.Fatal("dispatcher has no queued task")
	}
	task := d.tasks[0]
	d.tasks = d.tasks[1:]
	d.mu.Unlock()
	task()
}

func (d *manualExecutionDispatcher) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tasks)
}
