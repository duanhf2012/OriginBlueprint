package blueprint

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrTimerKeyEmpty         = errors.New("blueprint timer key is empty")
	ErrTimerKeyAlreadyExists = errors.New("blueprint timer key already exists")
	ErrTimerDurationInvalid  = errors.New("blueprint timer duration is invalid")
)

type ScheduledTimer interface{ Cancel() bool }

// TimerScheduler 可由宿主替换成自己的时间轮。callback 必须在 Schedule 返回后异步调用。
type TimerScheduler interface {
	Schedule(delay time.Duration, callback func()) (ScheduledTimer, error)
}

type systemTimerScheduler struct{}
type systemScheduledTimer struct{ timer *time.Timer }

func (systemTimerScheduler) Schedule(delay time.Duration, callback func()) (ScheduledTimer, error) {
	if callback == nil {
		return nil, fmt.Errorf("timer callback is nil")
	}
	return &systemScheduledTimer{timer: time.AfterFunc(delay, callback)}, nil
}
func (timer *systemScheduledTimer) Cancel() bool {
	return timer != nil && timer.timer != nil && timer.timer.Stop()
}

// SetTimerScheduler 设置 Delay 和 Timer 共用的调度器。传 nil 恢复系统时钟。
func (b *Blueprint) SetTimerScheduler(scheduler TimerScheduler) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.timerScheduler = scheduler
	b.mu.Unlock()
}
func (b *Blueprint) scheduler() TimerScheduler {
	if b == nil {
		return systemTimerScheduler{}
	}
	b.mu.RLock()
	scheduler := b.timerScheduler
	b.mu.RUnlock()
	if scheduler == nil {
		return systemTimerScheduler{}
	}
	return scheduler
}

type timerCaptureFrame struct {
	contexts  map[int]*ExecContext
	variables []IPort
}

func cloneExecContext(source *ExecContext) *ExecContext {
	if source == nil {
		return nil
	}
	return &ExecContext{InputPorts: clonePorts(source.InputPorts), OutputPorts: clonePorts(source.OutputPorts), ExecInputPortID: source.ExecInputPortID}
}

// captureTimerState 给每个 Timer 保存独立快照，之后的入口执行不会覆盖它。
func (g *Graph) captureTimerState() *timerCaptureFrame {
	frame := &timerCaptureFrame{}
	if g == nil {
		return frame
	}
	g.contextMu.Lock()
	frame.contexts = make(map[int]*ExecContext)
	for index, current := range g.context {
		if current != nil && current.state&execContextGenerationMask == g.contextGeneration {
			frame.contexts[index] = cloneExecContext(current)
		}
	}
	g.contextMu.Unlock()
	frame.variables = clonePorts(g.variables)
	return frame
}

func (g *Graph) restoreTimerCapture(frame *timerCaptureFrame) {
	if g == nil || frame == nil {
		return
	}
	g.contextMu.Lock()
	for index, source := range frame.contexts {
		if index < 0 || index >= len(g.context) || source == nil {
			continue
		}
		ctx := cloneExecContext(source)
		ctx.state = g.contextGeneration
		g.context[index] = ctx
	}
	g.contextMu.Unlock()
	if frame.variables != nil {
		g.variables = clonePorts(frame.variables)
	}
}

type instanceTimer struct {
	key        string
	generation uint64
	blueprint  *Blueprint
	instance   *GraphInstance
	compiled   *CompiledGraph
	target     VMTarget
	capture    *timerCaptureFrame
	interval   time.Duration
	looping    bool
	scheduled  ScheduledTimer
}

func checkedMilliseconds(value PortInt) (time.Duration, error) {
	if value < 0 || value > PortInt(math.MaxInt64/int64(time.Millisecond)) {
		return 0, ErrTimerDurationInvalid
	}
	return time.Duration(value) * time.Millisecond, nil
}

func (b *Blueprint) createTimer(graph *Graph, node *ExecNode, durationMs PortInt, looping bool, firstDelayMs PortInt) error {
	if b == nil || graph == nil || graph.instance == nil || node == nil {
		return fmt.Errorf("timer runtime is unavailable")
	}
	key := strings.TrimSpace(node.TimerKey)
	if key == "" {
		return ErrTimerKeyEmpty
	}
	interval, err := checkedMilliseconds(durationMs)
	if err != nil || looping && interval <= 0 {
		return fmt.Errorf("%w: %dms", ErrTimerDurationInvalid, durationMs)
	}
	delay := interval
	if firstDelayMs >= 0 {
		delay, err = checkedMilliseconds(firstDelayMs)
		if err != nil {
			return fmt.Errorf("%w: first delay %dms", ErrTimerDurationInvalid, firstDelayMs)
		}
	}
	if len(node.Next) <= 1 || node.Next[1] == nil {
		return fmt.Errorf("timer %q callback is not connected", key)
	}
	inputPortID := 0
	if len(node.NextInPort) > 1 {
		inputPortID = node.NextInPort[1]
	}
	timer := &instanceTimer{key: key, blueprint: b, instance: graph.instance, compiled: graph.compiled, target: VMTarget{PC: PC(node.Next[1].Index), InputPortID: inputPortID}, capture: graph.captureTimerState(), interval: interval, looping: looping}
	instance := graph.instance
	instance.lifecycleMu.Lock()
	if instance.released {
		instance.lifecycleMu.Unlock()
		return ErrGraphReleased
	}
	if instance.timers == nil {
		instance.timers = make(map[string]*instanceTimer)
	}
	if instance.timers[key] != nil {
		instance.lifecycleMu.Unlock()
		return fmt.Errorf("%w: %s", ErrTimerKeyAlreadyExists, key)
	}
	instance.timerSeq++
	timer.generation = instance.timerSeq
	instance.timers[key] = timer
	instance.lifecycleMu.Unlock()
	if err := timer.schedule(delay); err != nil {
		instance.lifecycleMu.Lock()
		if instance.timers[key] == timer {
			delete(instance.timers, key)
		}
		instance.lifecycleMu.Unlock()
		return err
	}
	return nil
}

func (timer *instanceTimer) schedule(delay time.Duration) error {
	scheduled, err := timer.blueprint.scheduler().Schedule(delay, timer.fire)
	if err != nil {
		return err
	}
	timer.instance.lifecycleMu.Lock()
	if timer.instance.released || timer.instance.timers[timer.key] != timer {
		timer.instance.lifecycleMu.Unlock()
		scheduled.Cancel()
		return nil
	}
	timer.scheduled = scheduled
	timer.instance.lifecycleMu.Unlock()
	return nil
}

func (timer *instanceTimer) fire() {
	if timer == nil || timer.instance == nil {
		return
	}
	timer.instance.lifecycleMu.Lock()
	if timer.instance.released || timer.instance.timers[timer.key] != timer {
		timer.instance.lifecycleMu.Unlock()
		return
	}
	timer.scheduled = nil
	if !timer.looping {
		delete(timer.instance.timers, timer.key)
	}
	timer.instance.lifecycleMu.Unlock()
	execution, err := timer.blueprint.startTimerCallback(timer)
	if err != nil || !timer.looping {
		if err != nil && timer.looping {
			timer.clearIfCurrent()
		}
		return
	}
	execution.addCompletionHook(func(*Execution) {
		timer.instance.lifecycleMu.Lock()
		active := !timer.instance.released && timer.instance.timers[timer.key] == timer
		timer.instance.lifecycleMu.Unlock()
		if active {
			if err := timer.schedule(timer.interval); err != nil {
				timer.clearIfCurrent()
			}
		}
	})
}

func (timer *instanceTimer) clearIfCurrent() {
	if timer == nil || timer.instance == nil {
		return
	}
	timer.instance.lifecycleMu.Lock()
	if timer.instance.timers[timer.key] == timer {
		delete(timer.instance.timers, timer.key)
	}
	timer.instance.lifecycleMu.Unlock()
}

func (i *GraphInstance) clearTimer(key string) bool {
	if i == nil {
		return false
	}
	key = strings.TrimSpace(key)
	i.lifecycleMu.Lock()
	timer := i.timers[key]
	var scheduled ScheduledTimer
	if timer != nil {
		delete(i.timers, key)
		scheduled = timer.scheduled
	}
	i.lifecycleMu.Unlock()
	if scheduled != nil {
		scheduled.Cancel()
	}
	return timer != nil
}

func (b *Blueprint) startTimerCallback(timer *instanceTimer) (*Execution, error) {
	if b == nil || timer == nil || timer.instance == nil || timer.compiled == nil {
		return nil, ErrGraphNotFound
	}
	b.mu.Lock()
	b.ensureLocked()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBlueprintClosed
	}
	if b.instances[timer.instance.graphID] != timer.instance {
		b.mu.Unlock()
		return nil, ErrGraphReleased
	}
	dispatcher := b.dispatcher
	if dispatcher == nil {
		dispatcher = defaultExecutionDispatcher
	}
	b.executionSeed++
	target := timer.target
	execution := &Execution{id: b.executionSeed, blueprint: b, graphID: timer.instance.graphID, instance: timer.instance, dispatcher: dispatcher, done: make(chan struct{}), state: ExecutionPending, callbackTarget: &target, callbackCapture: timer.capture}
	execution.scope = &executionScope{execution: execution, dispatcher: dispatcher, budget: newExecutionBudget(defaultExecutionStepLimit)}
	graph := NewGraph(timer.compiled)
	graph.name, graph.graphID, graph.module, graph.instance, graph.logger, graph.execution, graph.budget = timer.instance.name, timer.instance.graphID, b.module, timer.instance, b.logger, execution, execution.scope.budget
	if b.traceEnabled && b.traceLogger != nil {
		graph.trace = &blueprintTraceRuntime{logger: b.traceLogger, state: &blueprintTraceState{}}
	}
	execution.graph = graph
	b.executions[execution.id] = execution
	b.mu.Unlock()
	execution.watchContext(context.Background())
	if err := execution.submitInitial(execution.runInitial); err != nil {
		execution.finish(ExecutionFailed, nil, err)
		return nil, err
	}
	return execution, nil
}
