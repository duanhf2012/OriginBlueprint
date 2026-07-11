package blueprint

import (
	"context"
	"fmt"
	"time"
)

var blueprintRuntimeSeed uint64

type TimerHandle struct {
	BlueprintID uint64
	GraphID     int64
	TimerID     uint64
	Generation  uint64
}

func (h TimerHandle) Valid() bool {
	return h.BlueprintID != 0 && h.GraphID != 0 && h.TimerID != 0 && h.Generation != 0
}

type runtimeTimer struct {
	handle             TimerHandle
	functionID         string
	functionName       string
	args               []any
	interval           time.Duration
	deadline           time.Time
	remaining          time.Duration
	period             time.Duration
	looping            bool
	active             bool
	paused             bool
	scheduled          ScheduledTaskHandle
	scheduleGeneration uint64
	scheduler          TimerScheduler
	callback           *Execution
	callbackStarting   bool
	cancelStarting     bool
}

func cloneTimerArgs(args []any) []any {
	cloned := make([]any, len(args))
	for index, arg := range args {
		cloned[index] = cloneAnyValue(arg)
	}
	return cloned
}

func (b *Blueprint) setTimerByFunction(instance *GraphInstance, functionID, functionName string, args []any, interval, firstDelay time.Duration, looping bool) (TimerHandle, error) {
	if b == nil || instance == nil {
		return TimerHandle{}, ErrGraphReleased
	}
	if interval < 0 || firstDelay < 0 || (looping && interval <= 0) {
		return TimerHandle{}, fmt.Errorf("invalid timer interval=%s firstDelay=%s looping=%v", interval, firstDelay, looping)
	}
	b.mu.RLock()
	closed := b.closed
	runtimeID := b.runtimeID
	current := b.instances[instance.graphID]
	scheduler := b.scheduler
	if scheduler == nil {
		scheduler = defaultTimerScheduler
	}
	var functionGraph *CompiledGraph
	if current == instance && instance.state != nil && instance.state.compiled != nil {
		functionGraph = resolveFunctionGraph(instance.state.compiled.Functions, functionID, functionName)
	}
	if closed {
		b.mu.RUnlock()
		return TimerHandle{}, ErrBlueprintClosed
	}
	if current != instance {
		b.mu.RUnlock()
		return TimerHandle{}, ErrGraphReleased
	}
	if functionGraph == nil || functionGraph.Entrances[FunctionEntranceID] == nil {
		b.mu.RUnlock()
		return TimerHandle{}, fmt.Errorf("timer function %q not found", functionID)
	}
	instance.lifecycleMu.Lock()
	if instance.released {
		instance.lifecycleMu.Unlock()
		b.mu.RUnlock()
		return TimerHandle{}, ErrGraphReleased
	}
	instance.timerMu.Lock()
	instance.runtimeTimerID++
	id := instance.runtimeTimerID
	handle := TimerHandle{BlueprintID: runtimeID, GraphID: instance.graphID, TimerID: id, Generation: id}
	timer := &runtimeTimer{
		handle:       handle,
		functionID:   functionID,
		functionName: functionName,
		args:         cloneTimerArgs(args),
		interval:     interval,
		deadline:     time.Now().Add(firstDelay),
		period:       firstDelay,
		looping:      looping,
		active:       true,
		scheduler:    scheduler,
	}
	if instance.runtimeTimers == nil {
		instance.runtimeTimers = map[uint64]*runtimeTimer{}
	}
	instance.runtimeTimers[id] = timer
	instance.timerMu.Unlock()
	instance.lifecycleMu.Unlock()
	b.mu.RUnlock()
	if !b.scheduleRuntimeTimer(instance, timer, firstDelay) {
		b.clearTimer(handle, false)
		return TimerHandle{}, ErrSchedulerClosed
	}
	return handle, nil
}

func (b *Blueprint) scheduleRuntimeTimer(instance *GraphInstance, timer *runtimeTimer, delay time.Duration) bool {
	if delay < 0 {
		delay = 0
	}
	instance.timerMu.Lock()
	current := instance.runtimeTimers[timer.handle.TimerID]
	if current != timer || !timer.active || timer.paused {
		instance.timerMu.Unlock()
		return false
	}
	timer.scheduleGeneration++
	generation := timer.scheduleGeneration
	instance.timerMu.Unlock()

	scheduled, err := timer.scheduler.Schedule(delay, func() {
		b.fireRuntimeTimer(timer.handle, generation)
	})
	if err != nil {
		return false
	}
	instance.timerMu.Lock()
	current = instance.runtimeTimers[timer.handle.TimerID]
	if current != timer || timer.scheduleGeneration != generation || !timer.active || timer.paused {
		instance.timerMu.Unlock()
		timer.scheduler.Cancel(scheduled)
		return false
	}
	timer.scheduled = scheduled
	instance.timerMu.Unlock()
	return true
}

func (b *Blueprint) fireRuntimeTimer(handle TimerHandle, scheduleGeneration uint64) {
	instance := b.timerInstance(handle)
	if instance == nil {
		return
	}
	now := time.Now()
	instance.timerMu.Lock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || timer.scheduleGeneration != scheduleGeneration || !timer.active || timer.paused {
		instance.timerMu.Unlock()
		return
	}
	timer.scheduled = 0
	timer.scheduleGeneration++
	callbackRunning := timer.callbackStarting || (timer.callback != nil && !timer.callback.IsDone())
	looping := timer.looping
	if looping {
		nextDeadline := timer.deadline.Add(timer.interval)
		if !nextDeadline.After(now) {
			missed := now.Sub(nextDeadline)/timer.interval + 1
			nextDeadline = nextDeadline.Add(missed * timer.interval)
		}
		timer.deadline = nextDeadline
		timer.period = timer.interval
	} else {
		timer.active = false
	}
	functionID := timer.functionID
	functionName := timer.functionName
	args := cloneTimerArgs(timer.args)
	nextDelay := time.Duration(0)
	if looping {
		nextDelay = time.Until(timer.deadline)
	}
	if !callbackRunning {
		timer.callbackStarting = true
	}
	instance.timerMu.Unlock()

	if looping {
		b.scheduleRuntimeTimer(instance, timer, nextDelay)
	}
	if callbackRunning {
		return
	}
	execution, err := b.startFunctionExecution(context.Background(), instance, functionID, functionName, args...)
	if err != nil {
		instance.timerMu.Lock()
		timer.callbackStarting = false
		instance.timerMu.Unlock()
		b.clearTimer(handle, false)
		return
	}
	instance.timerMu.Lock()
	timer.callbackStarting = false
	registered := false
	cancelStarting := timer.cancelStarting
	if current := instance.runtimeTimers[handle.TimerID]; current == timer {
		timer.callback = execution
		registered = true
	}
	instance.timerMu.Unlock()
	if !registered {
		if cancelStarting {
			execution.Cancel()
		}
		return
	}
	execution.addCompletionHook(func(done *Execution) {
		b.timerCallbackCompleted(handle, done)
	})
}

func (b *Blueprint) timerCallbackCompleted(handle TimerHandle, execution *Execution) {
	instance := b.timerInstance(handle)
	if instance == nil {
		return
	}
	_, executionErr := execution.Result()
	var scheduled ScheduledTaskHandle
	var scheduler TimerScheduler
	instance.timerMu.Lock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || timer.callback != execution {
		instance.timerMu.Unlock()
		return
	}
	timer.callback = nil
	if executionErr != nil || !timer.looping {
		scheduled = timer.scheduled
		scheduler = timer.scheduler
		delete(instance.runtimeTimers, handle.TimerID)
	}
	instance.timerMu.Unlock()
	if scheduled != 0 {
		scheduler.Cancel(scheduled)
	}
}

func (b *Blueprint) timerInstance(handle TimerHandle) *GraphInstance {
	if b == nil || !handle.Valid() {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.runtimeID != handle.BlueprintID {
		return nil
	}
	return b.instances[handle.GraphID]
}

func (b *Blueprint) isTimerValid(handle TimerHandle) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	timer := instance.runtimeTimers[handle.TimerID]
	return timer != nil && timer.handle == handle && (timer.active || timer.paused)
}

func (b *Blueprint) clearTimer(handle TimerHandle, cancelRunning bool) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle {
		instance.timerMu.Unlock()
		return false
	}
	delete(instance.runtimeTimers, handle.TimerID)
	scheduled := timer.scheduled
	callback := timer.callback
	scheduler := timer.scheduler
	if cancelRunning && timer.callbackStarting {
		timer.cancelStarting = true
	}
	timer.active = false
	timer.paused = false
	timer.scheduleGeneration++
	instance.timerMu.Unlock()
	if scheduled != 0 {
		scheduler.Cancel(scheduled)
	}
	if cancelRunning && callback != nil {
		callback.Cancel()
	}
	return true
}

func (b *Blueprint) pauseTimer(handle TimerHandle) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || !timer.active || timer.paused {
		instance.timerMu.Unlock()
		return false
	}
	timer.remaining = time.Until(timer.deadline)
	if timer.remaining < 0 {
		timer.remaining = 0
	}
	timer.active = false
	timer.paused = true
	timer.scheduleGeneration++
	scheduled := timer.scheduled
	scheduler := timer.scheduler
	timer.scheduled = 0
	instance.timerMu.Unlock()
	if scheduled != 0 {
		scheduler.Cancel(scheduled)
	}
	return true
}

func (b *Blueprint) resumeTimer(handle TimerHandle) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || !timer.paused {
		instance.timerMu.Unlock()
		return false
	}
	delay := timer.remaining
	timer.deadline = time.Now().Add(delay)
	timer.active = true
	timer.paused = false
	instance.timerMu.Unlock()
	if !b.scheduleRuntimeTimer(instance, timer, delay) {
		b.clearTimer(handle, false)
		return false
	}
	return true
}

func (b *Blueprint) isTimerActive(handle TimerHandle) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	timer := instance.runtimeTimers[handle.TimerID]
	return timer != nil && timer.handle == handle && timer.active && !timer.paused
}

func (b *Blueprint) isTimerPaused(handle TimerHandle) bool {
	instance := b.timerInstance(handle)
	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	timer := instance.runtimeTimers[handle.TimerID]
	return timer != nil && timer.handle == handle && timer.paused
}

func (b *Blueprint) timerRemaining(handle TimerHandle) time.Duration {
	instance := b.timerInstance(handle)
	if instance == nil {
		return -1
	}
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || (!timer.active && !timer.paused) {
		return -1
	}
	if timer.paused {
		return timer.remaining
	}
	remaining := time.Until(timer.deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (b *Blueprint) timerElapsed(handle TimerHandle) time.Duration {
	instance := b.timerInstance(handle)
	if instance == nil {
		return -1
	}
	instance.timerMu.Lock()
	defer instance.timerMu.Unlock()
	timer := instance.runtimeTimers[handle.TimerID]
	if timer == nil || timer.handle != handle || (!timer.active && !timer.paused) {
		return -1
	}
	remaining := timer.remaining
	if !timer.paused {
		remaining = time.Until(timer.deadline)
	}
	elapsed := timer.period - remaining
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (b *Blueprint) startFunctionExecution(ctx context.Context, instance *GraphInstance, functionID, functionName string, args ...any) (*Execution, error) {
	b.mu.Lock()
	b.ensureLocked()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBlueprintClosed
	}
	if b.instances[instance.graphID] != instance || instance.state == nil || instance.state.compiled == nil {
		b.mu.Unlock()
		return nil, ErrGraphReleased
	}
	compiled := resolveFunctionGraph(instance.state.compiled.Functions, functionID, functionName)
	if compiled == nil || compiled.Entrances[FunctionEntranceID] == nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("function %s not found", functionID)
	}
	dispatcher := b.dispatcher
	if dispatcher == nil {
		dispatcher = defaultExecutionDispatcher
	}
	b.executionSeed++
	execution := &Execution{
		id:         b.executionSeed,
		blueprint:  b,
		graphID:    instance.graphID,
		instance:   instance,
		dispatcher: dispatcher,
		entranceID: FunctionEntranceID,
		args:       cloneTimerArgs(args),
		done:       make(chan struct{}),
		state:      ExecutionPending,
	}
	graph := NewGraph(compiled)
	graph.name = functionName
	graph.graphID = instance.graphID
	graph.module = instance.module
	graph.instance = instance
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

func (b *Blueprint) cancelInstanceRuntimeTimers(instance *GraphInstance) {
	if b == nil || instance == nil {
		return
	}
	instance.timerMu.Lock()
	type scheduledTimer struct {
		scheduler TimerScheduler
		handle    ScheduledTaskHandle
	}
	scheduled := make([]scheduledTimer, 0, len(instance.runtimeTimers))
	for _, timer := range instance.runtimeTimers {
		if timer.scheduled != 0 {
			scheduled = append(scheduled, scheduledTimer{scheduler: timer.scheduler, handle: timer.scheduled})
		}
		timer.active = false
		timer.paused = false
		timer.scheduleGeneration++
	}
	instance.runtimeTimers = nil
	instance.timerMu.Unlock()
	for _, task := range scheduled {
		task.scheduler.Cancel(task.handle)
	}
}
