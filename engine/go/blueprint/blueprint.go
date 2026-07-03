package blueprint

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Blueprint 是蓝图库的入口对象，负责加载编译图、创建实例并分发执行请求。
//
// 编译后的执行树由多个实例共享，实例自身只保存运行期上下文。
type Blueprint struct {
	mu            sync.RWMutex
	graphs        map[string]*CompiledGraph
	instances     map[int64]*GraphInstance
	execFactories []func() IExecNode
	module        IBlueprintModule
	cancelTimer   func(*uint64) bool
	logger        IBlueprintLogger
	traceEnabled  bool
	traceLogger   BlueprintTraceLogger
	execDefPath   string
	graphPath     string
	seedID        int64
}

// GraphInstance 保存单个 Create 实例的运行期状态。
type GraphInstance struct {
	name       string
	compiled   *CompiledGraph
	graphID    int64
	module     IBlueprintModule
	timers     map[uint64]struct{}
	timerMu    sync.Mutex
	variables  map[string]IPort
	variableMu sync.RWMutex
}

// HotReloadResult 描述一次热加载应用到运行时后的结果。
type HotReloadResult struct {
	GraphCount         int
	UpdatedInstances   int
	UnchangedInstances int
}

// hotReloadPlan 保存已经在后台完成解析和编译的新蓝图集合。
//
// 调用 apply 时只做短时间指针替换，适合投递回服务器主协程执行。
type hotReloadPlan struct {
	blueprint *Blueprint
	graphs    map[string]*CompiledGraph
	result    HotReloadResult
}

// apply 将已编译的新蓝图集合应用到运行时。
func (p *hotReloadPlan) apply() HotReloadResult {
	if p == nil || p.blueprint == nil {
		return HotReloadResult{}
	}
	b := p.blueprint
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	result := p.result
	if p.graphs == nil {
		return result
	}
	b.graphs = p.graphs
	for _, instance := range b.instances {
		if compiled := p.graphs[instance.name]; compiled != nil {
			instance.compiled = compiled
			result.UpdatedInstances++
			continue
		}
		result.UnchangedInstances++
	}
	return result
}

// AddCompiledGraph 手动加入一份已经编译完成的蓝图。
func (b *Blueprint) AddCompiledGraph(name string, graph *CompiledGraph) {
	if name == "" || graph == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	b.graphs[name] = graph
}

// Create 创建一个蓝图实例并返回实例 ID。
//
// 同名蓝图的多个实例共享 CompiledGraph，但变量和 timer 互相隔离。
func (b *Blueprint) Create(graphName string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	compiled := b.graphs[graphName]
	if compiled == nil {
		return 0
	}
	graphID := atomic.AddInt64(&b.seedID, 1)
	b.instances[graphID] = &GraphInstance{
		name:      graphName,
		compiled:  compiled,
		graphID:   graphID,
		module:    b.module,
		variables: initialVariables(compiled),
	}
	return graphID
}

// Do 从指定入口执行蓝图实例。
//
// 每次调用都会创建轻量 Graph 运行对象，复用实例上的共享变量上下文。
func (b *Blueprint) Do(graphID int64, entranceID int64, args ...any) (PortArray, error) {
	b.mu.RLock()
	instance := b.instances[graphID]
	if instance == nil {
		b.mu.RUnlock()
		return nil, nil
	}
	name := instance.name
	compiled := instance.compiled
	module := instance.module
	variables := instance.variables
	variableMu := &instance.variableMu
	traceEnabled := b.traceEnabled && b.traceLogger != nil
	traceLogger := b.traceLogger
	logger := b.logger
	b.mu.RUnlock()

	graph := NewGraph(compiled)
	graph.name = name
	graph.graphID = graphID
	graph.module = module
	graph.instance = instance
	graph.variables = variables
	graph.variableMu = variableMu
	graph.logger = logger
	if traceEnabled {
		graph.trace = &blueprintTraceRuntime{logger: traceLogger, state: &blueprintTraceState{}}
	}
	return graph.Do(entranceID, args...)
}

// TriggerEvent 兼容旧接口，用入口 ID 触发一次蓝图事件。
func (b *Blueprint) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	_, err := b.Do(graphID, eventID, args...)
	return err
}

// ReleaseGraph 释放实例并取消实例上仍然挂起的 timer。
func (b *Blueprint) ReleaseGraph(graphID int64) {
	b.mu.Lock()
	instance := b.instances[graphID]
	delete(b.instances, graphID)
	cancelTimer := b.cancelTimer
	b.mu.Unlock()

	if instance == nil {
		return
	}
	instance.timerMu.Lock()
	if len(instance.timers) == 0 {
		instance.timers = nil
		instance.timerMu.Unlock()
		return
	}
	timerIDs := make([]uint64, 0, len(instance.timers))
	for timerID := range instance.timers {
		timerIDs = append(timerIDs, timerID)
	}
	instance.timers = nil
	instance.timerMu.Unlock()

	for _, timerID := range timerIDs {
		id := timerID
		if instance.module != nil {
			instance.module.CancelTimerId(graphID, &id)
			continue
		}
		if cancelTimer != nil {
			cancelTimer(&id)
		}
	}
}

// CancelTimerId 取消指定实例上的 timer。
func (b *Blueprint) CancelTimerId(graphID int64, timerID *uint64) bool {
	if timerID == nil {
		return false
	}
	id := *timerID
	b.mu.RLock()
	module := b.module
	cancelTimer := b.cancelTimer
	instance := b.instances[graphID]
	b.mu.RUnlock()

	if module != nil {
		module.CancelTimerId(graphID, timerID)
	} else if cancelTimer != nil {
		cancelTimer(timerID)
	}

	if instance == nil {
		return false
	}
	instance.timerMu.Lock()
	delete(instance.timers, id)
	instance.timerMu.Unlock()
	return true
}

// prepareHotReload 在调用方协程中重新读取节点定义和蓝图文件，并编译为可应用的热加载计划。
//
// 该阶段不替换当前运行中的蓝图；只有应用返回计划时才会短锁替换关键指针。
func (b *Blueprint) prepareHotReload() (*hotReloadPlan, error) {
	b.mu.RLock()
	execDefPath := b.execDefPath
	graphPath := b.graphPath
	execFactories := append([]func() IExecNode(nil), b.execFactories...)
	b.mu.RUnlock()

	if execDefPath == "" || graphPath == "" {
		return &hotReloadPlan{blueprint: b}, nil
	}
	registry := NewRegistry()
	factories := append(BuiltinExecNodeFactories(), execFactories...)
	if err := loadDefinitionDir(registry, execDefPath, factories); err != nil {
		return nil, err
	}
	graphs, err := loadGraphDir(registry, graphPath)
	if err != nil {
		return nil, err
	}
	return &hotReloadPlan{
		blueprint: b,
		graphs:    graphs,
		result:    HotReloadResult{GraphCount: len(graphs)},
	}, nil
}

// HotReload 重新读取节点定义和蓝图文件，编译成功后短锁替换运行时关键指针。
//
// 该方法是线程安全的，可以直接放到业务协程或 goroutine 中调用；热加载期间并发 Do
// 会继续使用进入 Do 时取得的编译图，不会被中途替换。解析或编译失败时返回错误，并保留
// 当前正在使用的旧蓝图不变。
func (b *Blueprint) HotReload() (*HotReloadResult, error) {
	plan, err := b.prepareHotReload()
	if err != nil {
		return nil, err
	}
	result := plan.apply()
	return &result, nil
}

// GetLogger 返回初始化时传入的日志对象。
func (b *Blueprint) GetLogger() IBlueprintLogger {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.logger
}

// GetGraphName 返回实例绑定的蓝图名称。
func (b *Blueprint) GetGraphName(graphID int64) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	instance := b.instances[graphID]
	if instance == nil {
		return ""
	}
	return instance.name
}

func (b *Blueprint) ensure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
}

func (b *Blueprint) ensureLocked() {
	if b.graphs == nil {
		b.graphs = map[string]*CompiledGraph{}
	}
	if b.instances == nil {
		b.instances = map[int64]*GraphInstance{}
	}
}

func (b *Blueprint) mustGraph(graphID int64) (*GraphInstance, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	graph := b.instances[graphID]
	if graph == nil {
		return nil, fmt.Errorf("can not find graph:%d", graphID)
	}
	return graph, nil
}
