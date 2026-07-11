package blueprint

import (
	"errors"
	"runtime"
)

var ErrExecutionRejected = errors.New("golang blueprint execution rejected")

// ExecutionDispatcher 将蓝图执行任务与服务器调用 goroutine 解耦。
type ExecutionDispatcher interface {
	Submit(task func()) error
}

type workerExecutionDispatcher struct {
	tasks chan func()
}

func newWorkerExecutionDispatcher(workers, queueSize int) *workerExecutionDispatcher {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	d := &workerExecutionDispatcher{tasks: make(chan func(), queueSize)}
	for index := 0; index < workers; index++ {
		go func() {
			for task := range d.tasks {
				if task != nil {
					task()
				}
			}
		}()
	}
	return d
}

func (d *workerExecutionDispatcher) Submit(task func()) error {
	if d == nil || task == nil {
		return ErrExecutionRejected
	}
	select {
	case d.tasks <- task:
		return nil
	default:
		return ErrExecutionRejected
	}
}

var defaultExecutionDispatcher ExecutionDispatcher = newWorkerExecutionDispatcher(runtime.GOMAXPROCS(0), 65536)
