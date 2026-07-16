package blueprint

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
)

type verificationScheduledTask struct {
	deadline int64
	order    uint64
	resume   func() error
}

type verificationFakeClock struct {
	mu        sync.Mutex
	now       int64
	nextOrder uint64
	tasks     []verificationScheduledTask
}

func (c *verificationFakeClock) schedule(delayMs PortInt, resume func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if delayMs < 0 {
		delayMs = 0
	}
	c.nextOrder++
	c.tasks = append(c.tasks, verificationScheduledTask{deadline: c.now + int64(delayMs), order: c.nextOrder, resume: resume})
	sort.SliceStable(c.tasks, func(i, j int) bool {
		if c.tasks[i].deadline == c.tasks[j].deadline {
			return c.tasks[i].order < c.tasks[j].order
		}
		return c.tasks[i].deadline < c.tasks[j].deadline
	})
}

func (c *verificationFakeClock) advance(deltaMs int64) []error {
	c.mu.Lock()
	if deltaMs < 0 {
		c.mu.Unlock()
		return []error{fmt.Errorf("fake clock cannot advance by %dms", deltaMs)}
	}
	c.now += deltaMs
	readyCount := 0
	for readyCount < len(c.tasks) && c.tasks[readyCount].deadline <= c.now {
		readyCount++
	}
	ready := append([]verificationScheduledTask(nil), c.tasks[:readyCount]...)
	c.tasks = append([]verificationScheduledTask(nil), c.tasks[readyCount:]...)
	c.mu.Unlock()

	errors := make([]error, 0)
	for _, task := range ready {
		if err := task.resume(); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func (c *verificationFakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.tasks)
}

func (c *verificationFakeClock) advanceNext() (bool, []error) {
	c.mu.Lock()
	if len(c.tasks) == 0 {
		c.mu.Unlock()
		return false, nil
	}
	delta := c.tasks[0].deadline - c.now
	c.mu.Unlock()
	return true, c.advance(delta)
}

type verificationMockDelayNode struct {
	BaseExecNode
	clock *verificationFakeClock
}

func (n *verificationMockDelayNode) GetName() string { return "MockDelayAsync" }

func (n *verificationMockDelayNode) Exec() (int, error) {
	if n.clock == nil {
		return -1, fmt.Errorf("MockDelayAsync fake clock is not configured")
	}
	delayMs, _ := n.GetInPortInt(1)
	value, _ := n.GetInPortInt(2)
	tag, _ := n.GetInPortStr(3)
	handle, err := n.Yield(0)
	if err != nil {
		return -1, err
	}
	n.clock.schedule(delayMs, func() error { return handle.Resume(value, tag) })
	return -1, ErrExecutionSuspended
}

func TestVerificationAsyncAssetsRandomDifferential(t *testing.T) {
	t.Run("06_async_delay_resume.obp", testVerificationDelayLoopRandom)
	t.Run("07_async_rpc_resume_to.obp", testVerificationRPCRandom)
	t.Run("functions/14_async_delay_function.obpf", testVerificationDelayFunctionRandom)
}

func testVerificationDelayLoopRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071406)
	random := rand.New(rand.NewSource(seed))
	clock := &verificationFakeClock{}
	graphs := loadVerificationFixtureSetWithRegistry(t, verificationFixtureRegistryWithClock(t, clock))
	graph := graphs["异步 Delay 恢复验证"]
	if graph == nil {
		t.Fatal("异步 Delay 恢复验证 fixture was not loaded")
	}
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		objectID := PortInt(random.Intn(2001) - 1000)
		loopLimit := PortInt(random.Intn(7))
		delayMs := PortInt(random.Intn(1001))
		input := fmt.Sprintf("objectId=%d,loopLimit=%d,delayMs=%d", objectID, loopLimit, delayMs)
		assertVerificationRandomInputUnique(t, seen, "06", seed, caseIndex, input)
		want := referenceAsyncDelayResume(objectID, loopLimit)
		assertVerificationRepeated(t, "06", seed, caseIndex, input, func() (PortArray, error) {
			return runVerificationDelayExecution(t, graph, EntranceIDIntParam, clock, objectID, loopLimit, delayMs)
		}, want)
	}
}

func testVerificationRPCRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071407)
	random := rand.New(rand.NewSource(seed))
	graph := loadVerificationGraphWithRegistry(t, "07_async_rpc_resume_to.obp", verificationFixtureRegistry(t))
	want := PortArray{{IntVal: 314}, {IntVal: 503}, {StrVal: "mock rpc unavailable"}}
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		input := randomVerificationIntInput(random)
		inputText := formatIntInputs(input)
		assertVerificationRandomInputUnique(t, seen, "07", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "07", seed, caseIndex, inputText, func() (PortArray, error) {
			return runVerificationMockRPCGraph(t, graph, input)
		}, want)
	}
}

func testVerificationDelayFunctionRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071414)
	random := rand.New(rand.NewSource(seed))
	clock := &verificationFakeClock{}
	graphs := loadVerificationFixtureSetWithRegistry(t, verificationFixtureRegistryWithClock(t, clock))
	function := verificationFixtureFunction(t, graphs, "functions/14_async_delay_function.obpf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		delayMs := PortInt(random.Intn(1001))
		value := PortInt(random.Intn(2001) - 1000)
		tag := PortString(fmt.Sprintf("delay-case-%d-%d", caseIndex, random.Intn(1_000_000)))
		input := fmt.Sprintf("delayMs=%d,value=%d,tag=%q", delayMs, value, tag)
		assertVerificationRandomInputUnique(t, seen, "14", seed, caseIndex, input)
		assertVerificationRepeated(t, "14", seed, caseIndex, input, func() (PortArray, error) {
			return runVerificationDelayExecution(t, function, FunctionEntranceID, clock, delayMs, value, tag)
		}, PortArray{{IntVal: value}, {StrVal: tag}})
	}
}

func runVerificationDelayExecution(t *testing.T, graph *CompiledGraph, entranceID int64, clock *verificationFakeClock, args ...any) (PortArray, error) {
	t.Helper()
	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("delay-verification", graph)
	execution, err := bp.Start(context.Background(), bp.Create("delay-verification"), entranceID, args...)
	if err != nil {
		return nil, err
	}
	for !execution.IsDone() {
		for dispatcher.len() > 0 {
			dispatcher.runNext(t)
		}
		if execution.IsDone() {
			break
		}
		advanced, resumeErrors := clock.advanceNext()
		if !advanced {
			return nil, fmt.Errorf("execution state=%v has no pending fake-clock task", execution.State())
		}
		if len(resumeErrors) != 0 {
			return nil, fmt.Errorf("fake-clock resume: %v", resumeErrors)
		}
	}
	return execution.Result()
}

func runVerificationMockRPCGraph(t *testing.T, graph *CompiledGraph, input [3]PortInt) (PortArray, error) {
	t.Helper()
	dispatcher := &manualExecutionDispatcher{}
	verificationMockRPCPending.Lock()
	verificationMockRPCPending.calls = nil
	verificationMockRPCPending.Unlock()
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("rpc-verification", graph)
	execution, err := bp.Start(context.Background(), bp.Create("rpc-verification"), EntranceIDIntParam, input[0], input[1], input[2])
	if err != nil {
		return nil, err
	}
	for !execution.IsDone() {
		for dispatcher.len() > 0 {
			dispatcher.runNext(t)
		}
		if execution.IsDone() {
			break
		}
		resumeVerificationMockRPC(t)
	}
	return execution.Result()
}

func TestVerificationDelayFunctionDeadlineOrderAndCancellation(t *testing.T) {
	clock := &verificationFakeClock{}
	graphs := loadVerificationFixtureSetWithRegistry(t, verificationFixtureRegistryWithClock(t, clock))
	function := verificationFixtureFunction(t, graphs, "functions/14_async_delay_function.obpf")
	dispatcher := &manualExecutionDispatcher{}
	bp := &Blueprint{}
	bp.SetExecutionDispatcher(dispatcher)
	bp.AddCompiledGraph("delay-order", function)
	graphID := bp.Create("delay-order")
	longExecution, err := bp.Start(context.Background(), graphID, FunctionEntranceID, PortInt(30), PortInt(30), PortString("long"))
	if err != nil {
		t.Fatal(err)
	}
	shortExecution, err := bp.Start(context.Background(), graphID, FunctionEntranceID, PortInt(10), PortInt(10), PortString("short"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	dispatcher.runNext(t)
	if errors := clock.advance(10); len(errors) != 0 {
		t.Fatalf("advance 10ms: %v", errors)
	}
	dispatcher.runNext(t)
	if !shortExecution.IsDone() || longExecution.IsDone() {
		t.Fatalf("after 10ms short/long done=%v/%v", shortExecution.IsDone(), longExecution.IsDone())
	}
	shortResult, err := shortExecution.Result()
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, shortResult, PortArray{{IntVal: 10}, {StrVal: "short"}})
	if errors := clock.advance(20); len(errors) != 0 {
		t.Fatalf("advance 20ms: %v", errors)
	}
	dispatcher.runNext(t)
	longResult, err := longExecution.Result()
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, longResult, PortArray{{IntVal: 30}, {StrVal: "long"}})

	cancelExecution, err := bp.Start(context.Background(), graphID, FunctionEntranceID, PortInt(5000), PortInt(5000), PortString("must-not-resume"))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.runNext(t)
	if !cancelExecution.Cancel() {
		t.Fatal("cancel execution returned false")
	}
	resumeErrors := clock.advance(5000)
	if len(resumeErrors) != 1 || !errors.Is(resumeErrors[0], ErrExecutionCanceled) {
		t.Fatalf("cancel resume errors=%v, want ErrExecutionCanceled", resumeErrors)
	}
	if dispatcher.len() != 0 {
		t.Fatalf("canceled execution queued %d dispatcher tasks", dispatcher.len())
	}
	if _, err := cancelExecution.Result(); !errors.Is(err, ErrExecutionCanceled) {
		t.Fatalf("cancel result error=%v, want ErrExecutionCanceled", err)
	}
}

func referenceAsyncDelayResume(objectID, loopLimit PortInt) PortArray {
	returns := make(PortArray, 0)
	for outer := PortInt(0); outer < loopLimit; outer++ {
		for _, value := range []PortInt{2, 4, 6} {
			returns = append(returns, ArrayData{IntVal: value}, ArrayData{StrVal: "nested-loop"})
		}
	}
	returns = append(returns, ArrayData{StrVal: "nested-loop:completed"})
	for index := PortInt(0); index < 3; index++ {
		returns = append(returns, ArrayData{IntVal: index}, ArrayData{StrVal: "foreach-any"})
	}
	returns = append(returns, ArrayData{StrVal: "foreach-any:completed"})
	for index := PortInt(0); index < loopLimit && index <= 1; index++ {
		returns = append(returns, ArrayData{IntVal: index}, ArrayData{StrVal: "break-loop"})
	}
	returns = append(returns, ArrayData{StrVal: "break-loop:completed"})
	for index := PortInt(0); index < loopLimit; index++ {
		returns = append(returns, ArrayData{IntVal: index}, ArrayData{StrVal: "while-loop"})
	}
	returns = append(returns,
		ArrayData{StrVal: "while-loop:completed"},
		ArrayData{IntVal: objectID},
		ArrayData{StrVal: "function-delay"},
	)
	return returns
}
