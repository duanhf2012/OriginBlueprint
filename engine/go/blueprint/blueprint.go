package blueprint

import (
	"context"
	"fmt"
	"strings"
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
	logger        IBlueprintLogger
	traceEnabled  bool
	traceLogger   BlueprintTraceLogger
	execDefPath   string
	graphPath     string
	seedID        int64
	dispatcher    ExecutionDispatcher
	executions    map[uint64]*Execution
	executionSeed uint64
	closed        bool
}

// GraphInstance 保存单个 Create 实例的运行期状态。
type GraphInstance struct {
	name        string
	graphID     int64
	module      IBlueprintModule
	state       *instanceRuntimeState
	lifecycleMu sync.Mutex
	released    bool
	releasedCh  chan struct{}
	leases      int
}

type instanceRuntimeState struct {
	compiled   *CompiledGraph
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
			instance.state = migrateInstanceRuntimeState(instance.state, compiled)
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
	ensureVMProgram(graph)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	b.graphs[name] = graph
}

// Create 创建一个蓝图实例并返回实例 ID。
//
// 同名蓝图的多个实例共享 CompiledGraph，但变量互相隔离。
func (b *Blueprint) Create(graphName string) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ensureLocked()
	if b.closed {
		return 0
	}
	compiled := b.graphs[graphName]
	if compiled == nil {
		return 0
	}
	graphID := atomic.AddInt64(&b.seedID, 1)
	b.instances[graphID] = &GraphInstance{
		name:       graphName,
		graphID:    graphID,
		module:     b.module,
		state:      newInstanceRuntimeState(compiled),
		releasedCh: make(chan struct{}),
	}
	return graphID
}

// Close 关闭 Blueprint，并取消全部实例和仍未完成的执行。
func (b *Blueprint) Close() error {
	b.mu.Lock()
	b.ensureLocked()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	instances := make([]*GraphInstance, 0, len(b.instances))
	for _, instance := range b.instances {
		instances = append(instances, instance)
	}
	executions := make([]*Execution, 0, len(b.executions))
	for _, execution := range b.executions {
		executions = append(executions, execution)
	}
	b.instances = map[int64]*GraphInstance{}
	b.executions = map[uint64]*Execution{}
	b.mu.Unlock()

	for _, instance := range instances {
		instance.markReleased()
	}
	for _, execution := range executions {
		execution.cancelWith(ErrBlueprintClosed)
	}
	return nil
}

// Do 从指定入口执行蓝图实例。
//
// 每次调用都会创建轻量 Graph 运行对象，复用实例上的共享变量上下文。
func (b *Blueprint) Do(graphID int64, entranceID int64, args ...any) (PortArray, error) {
	return b.DoContext(context.Background(), graphID, entranceID, args...)
}

func newInstanceRuntimeState(compiled *CompiledGraph) *instanceRuntimeState {
	return &instanceRuntimeState{compiled: compiled, variables: initialVariables(compiled)}
}

func migrateInstanceRuntimeState(old *instanceRuntimeState, compiled *CompiledGraph) *instanceRuntimeState {
	next := newInstanceRuntimeState(compiled)
	if old == nil || old.compiled == nil || compiled == nil {
		return next
	}
	old.variableMu.RLock()
	defer old.variableMu.RUnlock()
	for name, config := range compiled.Variables {
		oldConfig, exists := old.compiled.Variables[name]
		if !exists || normalizeVariableType(oldConfig.Type) != normalizeVariableType(config.Type) {
			continue
		}
		if value := old.variables[name]; value != nil {
			next.variables[name] = value.Clone()
		}
	}
	return next
}

func normalizeVariableType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "int", "integer":
		return "integer"
	case "str", "string":
		return "string"
	case "bool", "boolean":
		return "boolean"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// TriggerEvent 兼容旧接口，用入口 ID 触发一次蓝图事件。
func (b *Blueprint) TriggerEvent(graphID int64, eventID int64, args ...any) error {
	_, err := b.Do(graphID, eventID, args...)
	return err
}

// ReleaseGraph 释放实例并取消仍未完成的执行。
func (b *Blueprint) ReleaseGraph(graphID int64) {
	b.mu.Lock()
	instance := b.instances[graphID]
	delete(b.instances, graphID)
	executions := make([]*Execution, 0)
	for _, execution := range b.executions {
		if execution.graphID == graphID {
			executions = append(executions, execution)
		}
	}
	b.mu.Unlock()

	if instance == nil {
		return
	}
	instance.markReleased()
	for _, execution := range executions {
		execution.cancelWith(ErrGraphReleased)
	}

}

func (i *GraphInstance) tryAcquireLease() bool {
	if i == nil {
		return false
	}
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()
	if i.released {
		return false
	}
	i.leases++
	return true
}

func (i *GraphInstance) releaseLease() {
	if i == nil {
		return
	}
	i.lifecycleMu.Lock()
	if i.leases > 0 {
		i.leases--
	}
	i.lifecycleMu.Unlock()
}

func (i *GraphInstance) markReleased() {
	if i == nil {
		return
	}
	i.lifecycleMu.Lock()
	if !i.released {
		i.released = true
		if i.releasedCh == nil {
			i.releasedCh = make(chan struct{})
		}
		close(i.releasedCh)
	}
	i.lifecycleMu.Unlock()
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
	if b.executions == nil {
		b.executions = map[uint64]*Execution{}
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
