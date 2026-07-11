package blueprint

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrExecutionPending  = errors.New("golang blueprint execution pending")
	ErrExecutionCanceled = errors.New("golang blueprint execution canceled")
	ErrBlueprintClosed   = errors.New("golang blueprint closed")
	ErrGraphNotFound     = errors.New("golang blueprint graph not found")
	ErrEntranceNotFound  = errors.New("golang blueprint entrance not found")
)

type ExecutionState uint8

const (
	ExecutionPending ExecutionState = iota
	ExecutionRunning
	ExecutionSuspended
	ExecutionCompleted
	ExecutionCanceled
	ExecutionFailed
)

func (s ExecutionState) terminal() bool {
	return s == ExecutionCompleted || s == ExecutionCanceled || s == ExecutionFailed
}

// Execution 是一次入口调用的独立运行句柄。
type Execution struct {
	id         uint64
	blueprint  *Blueprint
	graphID    int64
	instance   *GraphInstance
	graph      *Graph
	dispatcher ExecutionDispatcher
	entranceID int64
	args       []any
	done       chan struct{}

	mu              sync.RWMutex
	state           ExecutionState
	result          PortArray
	err             error
	cancelErr       error
	stopContext     func() bool
	cancelSeq       uint64
	cancelHooks     map[uint64]func()
	pending         *Continuation
	pendingNext     int
	pendingArgs     []any
	completionHooks []func(*Execution)
}

func (e *Execution) ID() uint64 { return e.id }

func (e *Execution) Done() <-chan struct{} { return e.done }

func (e *Execution) State() ExecutionState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

func (e *Execution) IsDone() bool { return e.State().terminal() }

func (e *Execution) Result() (PortArray, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.state.terminal() {
		return nil, ErrExecutionPending
	}
	return append(PortArray(nil), e.result...), e.err
}

func (e *Execution) Cancel() bool {
	return e.requestCancel(ErrExecutionCanceled)
}

func (e *Execution) cancelWith(err error) bool {
	if err == nil {
		err = ErrExecutionCanceled
	}
	return e.requestCancel(err)
}

func (e *Execution) requestCancel(err error) bool {
	e.mu.Lock()
	if e.state.terminal() || e.cancelErr != nil {
		e.mu.Unlock()
		return false
	}
	e.cancelErr = err
	running := e.state == ExecutionRunning
	cancelHooks := make([]func(), 0, len(e.cancelHooks))
	for _, cancelHook := range e.cancelHooks {
		cancelHooks = append(cancelHooks, cancelHook)
	}
	e.cancelHooks = nil
	e.mu.Unlock()
	for _, cancelHook := range cancelHooks {
		cancelHook()
	}
	if running {
		return true
	}
	return e.finish(ExecutionCanceled, nil, err)
}

func (e *Execution) cancellationError() error {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cancelErr
}

func (e *Execution) runInitial() {
	if !e.beginRun() {
		return
	}
	if !e.instance.tryAcquireLease() {
		e.finish(ExecutionFailed, nil, ErrGraphReleased)
		return
	}
	var returns PortArray
	var err error
	func() {
		defer e.instance.releaseLease()
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("blueprint execution panic: %v", recovered)
			}
		}()
		returns, err = e.graph.runEntrance(e.entranceID, e.args...)
	}()
	e.finishRun(returns, err)
}

func (e *Execution) beginRun() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.terminal() {
		return false
	}
	e.state = ExecutionRunning
	return true
}

func (e *Execution) scheduleContinuation(c *Continuation, args ...any) error {
	return e.scheduleContinuationAt(c, c.nextIndex, args...)
}

func (e *Execution) scheduleContinuationAt(c *Continuation, nextIndex int, args ...any) error {
	if err := c.reserve(); err != nil {
		return err
	}
	e.mu.Lock()
	if e.cancelErr != nil {
		err := e.cancelErr
		e.mu.Unlock()
		return err
	}
	switch e.state {
	case ExecutionRunning:
		if e.pending != nil {
			e.mu.Unlock()
			return ErrExecutionPending
		}
		e.pending = c
		e.pendingNext = nextIndex
		e.pendingArgs = append([]any(nil), args...)
		e.mu.Unlock()
		return nil
	case ExecutionSuspended:
		e.state = ExecutionPending
		e.mu.Unlock()
		return e.submitReservedContinuation(c, nextIndex, args...)
	default:
		state := e.state
		stateErr := e.err
		e.mu.Unlock()
		if state == ExecutionCanceled {
			return ErrExecutionCanceled
		}
		if state.terminal() {
			return stateErr
		}
		return ErrExecutionPending
	}
}

func (e *Execution) submitReservedContinuation(c *Continuation, nextIndex int, args ...any) error {
	if err := e.dispatcher.Submit(func() {
		if !e.beginRun() {
			return
		}
		var err error
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("blueprint continuation panic: %v", recovered)
				}
			}()
			err = c.resumeReservedAt(nextIndex, args...)
		}()
		e.finishRun(e.graph.resultSnapshot(), err)
	}); err != nil {
		e.finish(ExecutionFailed, nil, err)
		return err
	}
	return nil
}

func (e *Execution) finishRun(result PortArray, err error) {
	if cancelErr := e.cancellationError(); cancelErr != nil {
		e.finish(ExecutionCanceled, nil, cancelErr)
		return
	}
	if errors.Is(err, ErrExecutionSuspended) {
		e.mu.Lock()
		if !e.state.terminal() {
			if e.pending != nil {
				continuation := e.pending
				nextIndex := e.pendingNext
				args := e.pendingArgs
				e.pending = nil
				e.pendingNext = -1
				e.pendingArgs = nil
				e.state = ExecutionPending
				e.mu.Unlock()
				if submitErr := e.submitReservedContinuation(continuation, nextIndex, args...); submitErr != nil {
					e.finish(ExecutionFailed, nil, submitErr)
				}
				return
			}
			e.state = ExecutionSuspended
		}
		e.mu.Unlock()
		return
	}
	if errors.Is(err, ErrFunctionReturned) {
		err = nil
	}
	if err != nil {
		e.finish(ExecutionFailed, result, err)
		return
	}
	e.finish(ExecutionCompleted, result, nil)
}

func (e *Execution) finish(state ExecutionState, result PortArray, err error) bool {
	e.mu.Lock()
	if e.state.terminal() {
		e.mu.Unlock()
		return false
	}
	e.state = state
	e.result = append(PortArray(nil), result...)
	e.err = err
	stopContext := e.stopContext
	e.stopContext = nil
	cancelHooks := make([]func(), 0, len(e.cancelHooks))
	for _, cancelHook := range e.cancelHooks {
		cancelHooks = append(cancelHooks, cancelHook)
	}
	e.cancelHooks = nil
	e.pending = nil
	e.pendingNext = -1
	e.pendingArgs = nil
	completionHooks := append([]func(*Execution){}, e.completionHooks...)
	e.completionHooks = nil
	e.mu.Unlock()
	if stopContext != nil {
		stopContext()
	}
	for _, cancelHook := range cancelHooks {
		cancelHook()
	}
	if e.blueprint != nil {
		e.blueprint.removeExecution(e.id)
	}
	close(e.done)
	for _, completionHook := range completionHooks {
		completionHook(e)
	}
	return true
}

func (e *Execution) addCancelHook(cancelHook func()) uint64 {
	if e == nil || cancelHook == nil {
		return 0
	}
	e.mu.Lock()
	if e.state.terminal() {
		e.mu.Unlock()
		cancelHook()
		return 0
	}
	e.cancelSeq++
	if e.cancelHooks == nil {
		e.cancelHooks = map[uint64]func(){}
	}
	e.cancelHooks[e.cancelSeq] = cancelHook
	id := e.cancelSeq
	e.mu.Unlock()
	return id
}

func (e *Execution) removeCancelHook(id uint64) {
	if e == nil || id == 0 {
		return
	}
	e.mu.Lock()
	delete(e.cancelHooks, id)
	e.mu.Unlock()
}

func (e *Execution) addCompletionHook(hook func(*Execution)) {
	if e == nil || hook == nil {
		return
	}
	e.mu.Lock()
	if e.state.terminal() {
		done := e.done
		e.mu.Unlock()
		<-done
		hook(e)
		return
	}
	e.completionHooks = append(e.completionHooks, hook)
	e.mu.Unlock()
}

func (e *Execution) watchContext(ctx context.Context) {
	stopContext := context.AfterFunc(ctx, func() { e.cancelWith(ctx.Err()) })
	e.mu.Lock()
	if e.state.terminal() {
		e.mu.Unlock()
		stopContext()
		return
	}
	e.stopContext = stopContext
	e.mu.Unlock()
}

func (b *Blueprint) Start(ctx context.Context, graphID int64, entranceID int64, args ...any) (*Execution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.ensureLocked()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBlueprintClosed
	}
	instance := b.instances[graphID]
	if instance == nil || instance.state == nil || instance.state.compiled == nil {
		b.mu.Unlock()
		return nil, ErrGraphNotFound
	}
	state := instance.state
	if state.compiled.Entrances[entranceID] == nil {
		b.mu.Unlock()
		return nil, ErrEntranceNotFound
	}
	dispatcher := b.dispatcher
	if dispatcher == nil {
		dispatcher = defaultExecutionDispatcher
	}
	b.executionSeed++
	execution := &Execution{
		id:         b.executionSeed,
		blueprint:  b,
		graphID:    graphID,
		instance:   instance,
		dispatcher: dispatcher,
		entranceID: entranceID,
		args:       append([]any(nil), args...),
		done:       make(chan struct{}),
		state:      ExecutionPending,
	}
	graph := NewGraph(state.compiled)
	graph.name = instance.name
	graph.graphID = graphID
	graph.module = instance.module
	graph.instance = instance
	graph.variables = state.variables
	graph.variableMu = &state.variableMu
	graph.logger = b.logger
	graph.execution = execution
	if b.traceEnabled && b.traceLogger != nil {
		graph.trace = &blueprintTraceRuntime{logger: b.traceLogger, state: &blueprintTraceState{}}
	}
	execution.graph = graph
	b.executions[execution.id] = execution
	b.mu.Unlock()

	execution.watchContext(ctx)
	if err := dispatcher.Submit(execution.runInitial); err != nil {
		execution.finish(ExecutionFailed, nil, err)
		return nil, err
	}
	return execution, nil
}

func (b *Blueprint) DoContext(ctx context.Context, graphID int64, entranceID int64, args ...any) (PortArray, error) {
	execution, err := b.Start(ctx, graphID, entranceID, args...)
	if err != nil {
		return nil, err
	}
	select {
	case <-execution.Done():
		return execution.Result()
	case <-ctx.Done():
		execution.cancelWith(ctx.Err())
		<-execution.Done()
		return execution.Result()
	}
}

func (b *Blueprint) SetExecutionDispatcher(dispatcher ExecutionDispatcher) {
	b.mu.Lock()
	b.dispatcher = dispatcher
	b.mu.Unlock()
}

func (b *Blueprint) SetTimerScheduler(scheduler TimerScheduler) {
	b.mu.Lock()
	b.scheduler = scheduler
	b.mu.Unlock()
}

func (b *Blueprint) timerScheduler() TimerScheduler {
	b.mu.RLock()
	scheduler := b.scheduler
	b.mu.RUnlock()
	if scheduler == nil {
		return defaultTimerScheduler
	}
	return scheduler
}

func (b *Blueprint) activeExecutionCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.executions)
}

func (b *Blueprint) removeExecution(id uint64) {
	b.mu.Lock()
	delete(b.executions, id)
	b.mu.Unlock()
}
