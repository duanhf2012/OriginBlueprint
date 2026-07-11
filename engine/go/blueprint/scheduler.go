package blueprint

import (
	"container/heap"
	"errors"
	"sync"
	"time"
)

var ErrSchedulerClosed = errors.New("golang blueprint timer scheduler closed")

type ScheduledTaskHandle uint64

type TimerScheduler interface {
	Schedule(delay time.Duration, callback func()) (ScheduledTaskHandle, error)
	Cancel(handle ScheduledTaskHandle) bool
}

type scheduledTask struct {
	handle   ScheduledTaskHandle
	deadline time.Time
	callback func()
	index    int
}

type scheduledTaskHeap []*scheduledTask

func (h scheduledTaskHeap) Len() int           { return len(h) }
func (h scheduledTaskHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h scheduledTaskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *scheduledTaskHeap) Push(value any) {
	task := value.(*scheduledTask)
	task.index = len(*h)
	*h = append(*h, task)
}
func (h *scheduledTaskHeap) Pop() any {
	old := *h
	last := len(old) - 1
	task := old[last]
	old[last] = nil
	task.index = -1
	*h = old[:last]
	return task
}

type sharedTimerScheduler struct {
	mu     sync.Mutex
	nextID uint64
	tasks  scheduledTaskHeap
	byID   map[ScheduledTaskHandle]*scheduledTask
	wake   chan struct{}
	stop   chan struct{}
	closed bool
}

func newSharedTimerScheduler() *sharedTimerScheduler {
	s := &sharedTimerScheduler{
		byID: map[ScheduledTaskHandle]*scheduledTask{},
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *sharedTimerScheduler) Schedule(delay time.Duration, callback func()) (ScheduledTaskHandle, error) {
	if s == nil || callback == nil || delay < 0 {
		return 0, ErrSchedulerClosed
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, ErrSchedulerClosed
	}
	s.nextID++
	task := &scheduledTask{
		handle:   ScheduledTaskHandle(s.nextID),
		deadline: time.Now().Add(delay),
		callback: callback,
	}
	heap.Push(&s.tasks, task)
	s.byID[task.handle] = task
	s.mu.Unlock()
	s.signalWake()
	return task.handle, nil
}

func (s *sharedTimerScheduler) Cancel(handle ScheduledTaskHandle) bool {
	if s == nil || handle == 0 {
		return false
	}
	s.mu.Lock()
	task := s.byID[handle]
	if task != nil {
		heap.Remove(&s.tasks, task.index)
		delete(s.byID, handle)
	}
	s.mu.Unlock()
	if task != nil {
		s.signalWake()
	}
	return task != nil
}

func (s *sharedTimerScheduler) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.stop)
	s.tasks = nil
	s.byID = map[ScheduledTaskHandle]*scheduledTask{}
	s.mu.Unlock()
	return nil
}

func (s *sharedTimerScheduler) signalWake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *sharedTimerScheduler) run() {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		if len(s.tasks) == 0 {
			s.mu.Unlock()
			select {
			case <-s.wake:
				continue
			case <-s.stop:
				return
			}
		}
		wait := time.Until(s.tasks[0].deadline)
		if wait <= 0 {
			now := time.Now()
			callbacks := make([]func(), 0, 1)
			for len(s.tasks) != 0 && !s.tasks[0].deadline.After(now) {
				task := heap.Pop(&s.tasks).(*scheduledTask)
				delete(s.byID, task.handle)
				callbacks = append(callbacks, task.callback)
			}
			s.mu.Unlock()
			for _, callback := range callbacks {
				func() {
					defer func() { _ = recover() }()
					callback()
				}()
			}
			continue
		}
		s.mu.Unlock()

		timer.Reset(wait)
		select {
		case <-timer.C:
		case <-s.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-s.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}
}

var defaultTimerScheduler TimerScheduler = newSharedTimerScheduler()
