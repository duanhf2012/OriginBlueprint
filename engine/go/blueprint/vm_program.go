package blueprint

// VMTarget 是一个 exec 边目标。
type VMTarget struct {
	PC          PC
	InputPortID int
}

// NodePlan 保存节点的只读编译信息。
type NodePlan struct {
	Node       *ExecNode
	Control    ControlKind
	Successors [][]VMTarget
}

// Program 是可由多个实例并发共享的只读 VM 程序。
type Program struct {
	Version       uint64
	Instructions  []Instruction
	Nodes         []NodePlan
	Entrances     map[int64]PC
	Functions     map[string]*Program
	Variables     map[string]VariableConfig
	FlowStackHint int
	LoopStackHint int
}
