package blueprint

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	ErrExecutionPending        = errors.New("golang blueprint execution pending")
	ErrExecutionCanceled       = errors.New("golang blueprint execution canceled")
	ErrExecutionCompleted      = errors.New("golang blueprint execution completed")
	ErrExecutionBudgetExceeded = errors.New("golang blueprint execution budget exceeded")
	ErrBlueprintClosed         = errors.New("golang blueprint closed")
	ErrGraphNotFound           = errors.New("golang blueprint graph not found")
	ErrEntranceNotFound        = errors.New("golang blueprint entrance not found")
	ErrGraphReleased           = errors.New("golang blueprint graph released")
)

const defaultExecutionStepLimit uint64 = 1_000_000
const maxExecutionCallDepth uint64 = 4_096

type executionBudget struct {
	limit uint64
	steps atomic.Uint64
	depth atomic.Uint64
}

func newExecutionBudget(limit uint64) *executionBudget {
	return &executionBudget{limit: limit}
}

func (b *executionBudget) consume() error {
	if b == nil {
		return nil
	}
	for {
		step := b.steps.Load()
		if step >= b.limit {
			return fmt.Errorf("%w: step limit %d", ErrExecutionBudgetExceeded, b.limit)
		}
		if b.steps.CompareAndSwap(step, step+1) {
			return nil
		}
	}
}

func (b *executionBudget) enter() error {
	if b == nil {
		return nil
	}
	if err := b.consume(); err != nil {
		return err
	}
	depth := b.depth.Add(1)
	if depth > maxExecutionCallDepth {
		b.depth.Add(^uint64(0))
		return fmt.Errorf("%w: call depth limit %d", ErrExecutionBudgetExceeded, maxExecutionCallDepth)
	}
	return nil
}

func (b *executionBudget) leave() {
	if b != nil {
		b.depth.Add(^uint64(0))
	}
}

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
	vm         *vmMachine
	dispatcher ExecutionDispatcher
	entranceID int64
	args       []any
	done       chan struct{}
	scope      *executionScope

	mu              sync.RWMutex
	state           ExecutionState
	result          PortArray
	err             error
	cancelErr       error
	stopContext     func() bool
	cancelSeq       uint64
	cancelHooks     map[uint64]func()
	completionHooks []func(*Execution)
}

// executionScope 保存一次执行的串行调度队列和预算。
type executionScope struct {
	execution  *Execution
	dispatcher ExecutionDispatcher
	budget     *executionBudget

	mu       sync.Mutex
	queue    []func()
	draining bool
	terminal atomic.Pointer[executionTerminal]
}

type executionTerminal struct{ err error }

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
	scope := e.ensureScope()
	e.mu.Lock()
	if e.state.terminal() || e.cancelErr != nil {
		e.mu.Unlock()
		return false
	}
	scope.markTerminal(err)
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
	if root := e.rootExecution(); root != e {
		return root.cancellationError()
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cancelErr
}

func (e *Execution) runInitial() {
	e.ensureScope()
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
		var found bool
		e.vm, found, err = e.graph.newVMMachineForEntrance(e.entranceID, e.args...)
		if err == nil && found {
			err = e.vm.run()
			returns = e.graph.resultSnapshot()
		}
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

func (e *Execution) finishRun(result PortArray, err error) {
	if cancelErr := e.cancellationError(); cancelErr != nil {
		e.finish(ExecutionCanceled, nil, cancelErr)
		return
	}
	if errors.Is(err, ErrExecutionSuspended) {
		e.mu.Lock()
		if !e.state.terminal() {
			e.state = ExecutionSuspended
		}
		e.mu.Unlock()
		return
	}
	if err != nil {
		e.finish(ExecutionFailed, result, err)
		return
	}
	e.finish(ExecutionCompleted, result, nil)
}

func (e *Execution) finish(state ExecutionState, result PortArray, err error) bool {
	return e.finishWhen(state, result, err, false)
}

func (e *Execution) finishSubmissionError(err error) bool {
	return e.finishWhen(ExecutionFailed, nil, err, true)
}

func (e *Execution) finishWhen(state ExecutionState, result PortArray, err error, submissionFailure bool) bool {
	scope := e.ensureScope()
	e.mu.Lock()
	if e.state.terminal() {
		e.mu.Unlock()
		return false
	}
	if submissionFailure && (e.cancelErr != nil || scope.terminalError() != nil) {
		e.mu.Unlock()
		return false
	}
	if !submissionFailure && e.cancelErr != nil && state != ExecutionCanceled {
		state = ExecutionCanceled
		result = nil
		err = e.cancelErr
	}
	terminalErr := err
	if terminalErr == nil {
		terminalErr = ErrExecutionCompleted
	}
	scope.markTerminal(terminalErr)
	e.state = state
	// result 由 Graph.resultSnapshot 创建，只属于当前 Execution，可以直接接管。
	e.result = result
	e.err = err
	stopContext := e.stopContext
	e.stopContext = nil
	cancelHooks := make([]func(), 0, len(e.cancelHooks))
	for _, cancelHook := range e.cancelHooks {
		cancelHooks = append(cancelHooks, cancelHook)
	}
	e.cancelHooks = nil
	completionHooks := append([]func(*Execution){}, e.completionHooks...)
	e.completionHooks = nil
	e.mu.Unlock()
	if stopContext != nil {
		stopContext()
	}
	for _, cancelHook := range cancelHooks {
		cancelHook()
	}
	if e.vm != nil {
		e.vm.release()
	} else if e.graph != nil {
		e.graph.releaseContextReferences()
	}
	if e.blueprint != nil {
		e.blueprint.removeExecution(e.id)
	}
	close(e.done)
	for _, completionHook := range completionHooks {
		completionHook(e)
	}
	e.mu.Lock()
	e.args = nil
	e.graph = nil
	e.vm = nil
	e.mu.Unlock()
	return true
}

func (e *Execution) addCancelHook(cancelHook func()) uint64 {
	if e == nil || cancelHook == nil {
		return 0
	}
	if root := e.rootExecution(); root != e {
		return root.addCancelHook(cancelHook)
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
	if root := e.rootExecution(); root != e {
		root.removeCancelHook(id)
		return
	}
	e.mu.Lock()
	delete(e.cancelHooks, id)
	e.mu.Unlock()
}

func (e *Execution) rootExecution() *Execution {
	if e != nil && e.scope != nil && e.scope.execution != nil {
		return e.scope.execution
	}
	return e
}

func (e *Execution) ensureScope() *executionScope {
	if e == nil {
		return nil
	}
	if root := e.rootExecution(); root != e {
		return root.ensureScope()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.scope != nil {
		return e.scope
	}
	dispatcher := e.dispatcher
	if dispatcher == nil {
		dispatcher = defaultExecutionDispatcher
	}
	e.scope = &executionScope{execution: e, dispatcher: dispatcher, budget: newExecutionBudget(defaultExecutionStepLimit)}
	return e.scope
}

func (e *Execution) submit(task func()) error {
	if e == nil {
		return fmt.Errorf("execution is nil")
	}
	return e.ensureScope().submit(task)
}

func (e *Execution) submitInitial(task func()) error {
	if e == nil {
		return fmt.Errorf("execution is nil")
	}
	return e.ensureScope().submitInitial(task)
}

func (s *executionScope) submit(task func()) error {
	return s.submitTask(task, false)
}

func (s *executionScope) submitInitial(task func()) error {
	return s.submitTask(task, true)
}

func (s *executionScope) submitTask(task func(), initial bool) error {
	if s == nil || task == nil {
		return fmt.Errorf("execution task is nil")
	}
	if err := s.terminalError(); err != nil {
		return err
	}
	s.mu.Lock()
	if err := s.terminalError(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.queue = append(s.queue, task)
	if s.draining {
		s.mu.Unlock()
		return nil
	}
	s.draining = true
	s.mu.Unlock()

	var err error
	if initial {
		if dispatcher, ok := s.dispatcher.(interface{ SubmitInitial(func()) error }); ok {
			err = dispatcher.SubmitInitial(s.runOne)
		} else {
			err = s.dispatcher.Submit(s.runOne)
		}
	} else {
		err = s.dispatcher.Submit(s.runOne)
	}
	if err != nil {
		s.mu.Lock()
		s.draining = false
		s.queue = nil
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *executionScope) terminalError() error {
	if s == nil {
		return nil
	}
	terminal := s.terminal.Load()
	if terminal == nil {
		return nil
	}
	return terminal.err
}

func (s *executionScope) markTerminal(err error) {
	if s == nil {
		return
	}
	if err == nil {
		err = ErrExecutionCompleted
	}
	if !s.terminal.CompareAndSwap(nil, &executionTerminal{err: err}) {
		return
	}
	s.mu.Lock()
	s.queue = nil
	s.mu.Unlock()
}

func (s *executionScope) runOne() {
	s.mu.Lock()
	if len(s.queue) == 0 {
		s.draining = false
		s.mu.Unlock()
		return
	}
	task := s.queue[0]
	s.queue[0] = nil
	s.queue = s.queue[1:]
	s.mu.Unlock()

	task()

	s.mu.Lock()
	if len(s.queue) == 0 {
		s.draining = false
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if err := s.dispatcher.Submit(s.runOne); err != nil {
		s.mu.Lock()
		s.draining = false
		s.queue = nil
		s.mu.Unlock()
		if s.execution != nil {
			s.execution.finishSubmissionError(err)
		}
	}
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
	if ctx == nil || ctx.Done() == nil {
		return
	}
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
	execution.scope = &executionScope{execution: execution, dispatcher: dispatcher, budget: newExecutionBudget(defaultExecutionStepLimit)}
	graph := NewGraph(state.compiled)
	graph.name = instance.name
	graph.graphID = graphID
	graph.module = instance.module
	graph.instance = instance
	graph.variables = state.variables
	graph.variableMu = &state.variableMu
	graph.logger = b.logger
	graph.execution = execution
	graph.budget = execution.scope.budget
	if b.traceEnabled && b.traceLogger != nil {
		graph.trace = &blueprintTraceRuntime{logger: b.traceLogger, state: &blueprintTraceState{}}
	}
	execution.graph = graph
	b.executions[execution.id] = execution
	b.mu.Unlock()

	execution.watchContext(ctx)
	if err := execution.submitInitial(execution.runInitial); err != nil {
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
