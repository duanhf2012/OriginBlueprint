package blueprint

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

const (
	verificationRandomCaseCount = 64
	verificationRepeatCount     = 3
	verificationSeedOffsetEnv   = "ORIGIN_BLUEPRINT_VERIFICATION_SEED_OFFSET"
)

func verificationRandomSeed(t *testing.T, base int64) int64 {
	t.Helper()
	rawOffset := os.Getenv(verificationSeedOffsetEnv)
	if rawOffset == "" {
		return base
	}
	offset, err := strconv.ParseInt(rawOffset, 10, 64)
	if err != nil {
		t.Fatalf("%s must be a signed 64-bit integer: %v", verificationSeedOffsetEnv, err)
	}
	return base + offset
}

func TestVerificationSynchronousAssetsRandomDifferential(t *testing.T) {
	t.Run("01_legacy_all_nodes_showcase.vgf", testVerificationLegacyRandom)
	t.Run("02_control_flow_maze.obp", testVerificationControlFlowRandom)
	t.Run("03_array_data_lab.obp", testVerificationArrayLabRandom)
	t.Run("04_deterministic_algorithm.obp", testVerificationAlgorithmRandom)
	t.Run("05_function_orchestrator.obp", testVerificationOrchestratorRandom)
	t.Run("functions/10_score_kernel.obpf", testVerificationScoreRandom)
	t.Run("functions/11_array_fold_and_format.obpf", testVerificationFoldRandom)
	t.Run("functions/12_nested_control_function.obpf", testVerificationNestedFunctionRandom)
	t.Run("functions/13_local_state_isolation.obpf", testVerificationLocalFunctionRandom)
	t.Run("functions/15_variable_types_lifecycle.obpf", testVerificationVariableTypesRandom)
}

func testVerificationLegacyRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071401)
	random := rand.New(rand.NewSource(seed))
	graph := loadVerificationGraph(t, "01_legacy_all_nodes_showcase.vgf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		objectID := PortInt(random.Intn(2001) - 1000)
		first := PortInt(random.Intn(201) - 100)
		second := PortInt(random.Intn(201) - 100)
		array := randomVerificationPortArray(random, random.Intn(9))
		assertVerificationRandomInputUnique(t, seen, "01", seed, caseIndex, fmt.Sprintf("objectId=%d,param1=%d,param2=%d,array=%s", objectID, first, second, formatPortArray(array)))
		assertVerificationRepeated(t, "01 integer entrance", seed, caseIndex, fmt.Sprintf("objectId=%d,param1=%d,param2=%d", objectID, first, second), func() (PortArray, error) {
			return NewGraph(graph).Do(EntranceIDIntParam, objectID, first, second)
		}, referenceLegacyShowcaseInteger())
		assertVerificationRepeated(t, "01 array entrance", seed, caseIndex, fmt.Sprintf("objectId=%d,array=%s", objectID, formatPortArray(array)), func() (PortArray, error) {
			return NewGraph(graph).Do(EntranceIDArrayParam, objectID, array)
		}, nil)
	}
}

func testVerificationControlFlowRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071402)
	random := rand.New(rand.NewSource(seed))
	graph := loadVerificationGraph(t, "02_control_flow_maze.obp")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		input := randomVerificationIntInput(random)
		inputText := formatIntInputs(input)
		assertVerificationRandomInputUnique(t, seen, "02", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "02", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(graph).Do(EntranceIDIntParam, input[0], input[1], input[2])
		}, referenceControlFlowMaze())
	}
}

func testVerificationArrayLabRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071403)
	random := rand.New(rand.NewSource(seed))
	graph := loadVerificationGraph(t, "03_array_data_lab.obp")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		objectID := PortInt(random.Intn(2001) - 1000)
		array := randomVerificationPortArray(random, random.Intn(9))
		inputText := fmt.Sprintf("objectId=%d,array=%s", objectID, formatPortArray(array))
		assertVerificationRandomInputUnique(t, seen, "03", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "03", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(graph).Do(EntranceIDArrayParam, objectID, array)
		}, referenceArrayDataLab())
	}
}

func testVerificationAlgorithmRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071404)
	random := rand.New(rand.NewSource(seed))
	graph := loadVerificationGraph(t, "04_deterministic_algorithm.obp")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		input := randomVerificationIntInput(random)
		inputText := formatIntInputs(input)
		assertVerificationRandomInputUnique(t, seen, "04", seed, caseIndex, inputText)
		want := referenceDeterministicAlgorithm(input[1], input[2])
		assertVerificationRepeated(t, "04", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(graph).Do(EntranceIDIntParam, input[0], input[1], input[2])
		}, want)
	}
}

func testVerificationOrchestratorRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071405)
	random := rand.New(rand.NewSource(seed))
	main := loadVerificationFixtureSet(t)["函数编排主图"]
	if main == nil {
		t.Fatal("函数编排主图 fixture was not loaded")
	}
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		input := randomVerificationIntInput(random)
		inputText := formatIntInputs(input)
		assertVerificationRandomInputUnique(t, seen, "05", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "05", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(main).Do(EntranceIDIntParam, input[0], input[1], input[2])
		}, referenceFunctionOrchestrator())
	}
}

func testVerificationScoreRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071410)
	random := rand.New(rand.NewSource(seed))
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/10_score_kernel.obpf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		base := PortInt(random.Intn(2001) - 1000)
		bonus := PortInt(random.Intn(2001) - 1000)
		multiplier := PortInt(random.Intn(33) - 16)
		inputText := fmt.Sprintf("base=%d,bonus=%d,multiplier=%d", base, bonus, multiplier)
		assertVerificationRandomInputUnique(t, seen, "10", seed, caseIndex, inputText)
		score, tier := referenceScoreKernel(base, bonus, multiplier)
		assertVerificationRepeated(t, "10", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(function).Do(FunctionEntranceID, base, bonus, multiplier)
		}, PortArray{{IntVal: score}, {StrVal: tier}})
	}
}

func testVerificationFoldRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071411)
	random := rand.New(rand.NewSource(seed))
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/11_array_fold_and_format.obpf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		items := randomVerificationPortArray(random, random.Intn(8)+2)
		weight := PortInt(random.Intn(33) - 16)
		inputText := fmt.Sprintf("items=%s,weight=%d", formatPortArray(items), weight)
		assertVerificationRandomInputUnique(t, seen, "11", seed, caseIndex, inputText)
		plain := make([]PortInt, len(items))
		for index, item := range items {
			plain[index] = item.IntVal
		}
		sum, summary := referenceArrayFoldAndFormat(plain, weight)
		assertVerificationRepeated(t, "11", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(function).Do(FunctionEntranceID, items, weight)
		}, PortArray{{IntVal: sum}, {StrVal: summary}})
	}
}

func testVerificationNestedFunctionRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071412)
	random := rand.New(rand.NewSource(seed))
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/12_nested_control_function.obpf")
	count, trace := referenceNestedControlFlow()
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	combinations := random.Perm(17 * 17)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		start := PortInt(combinations[caseIndex]/17 - 8)
		limit := start + PortInt(combinations[caseIndex]%17)
		inputText := fmt.Sprintf("start=%d,limit=%d", start, limit)
		assertVerificationRandomInputUnique(t, seen, "12", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "12", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(function).Do(FunctionEntranceID, start, limit)
		}, PortArray{{IntVal: count}, {StrVal: trace}})
	}
}

func testVerificationLocalFunctionRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071413)
	random := rand.New(rand.NewSource(seed))
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/13_local_state_isolation.obpf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	seedValues := random.Perm(2001)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		seedValue := PortInt(seedValues[caseIndex] - 1000)
		inputText := fmt.Sprintf("seed=%d", seedValue)
		assertVerificationRandomInputUnique(t, seen, "13", seed, caseIndex, inputText)
		assertVerificationRepeated(t, "13", seed, caseIndex, inputText, func() (PortArray, error) {
			return NewGraph(function).Do(FunctionEntranceID, seedValue)
		}, PortArray{{IntVal: referenceLocalStateIsolation(seedValue)}})
	}
}

func testVerificationVariableTypesRandom(t *testing.T) {
	seed := verificationRandomSeed(t, 2026071415)
	random := rand.New(rand.NewSource(seed))
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/15_variable_types_lifecycle.obpf")
	seen := make(map[string]struct{}, verificationRandomCaseCount)
	for caseIndex := 0; caseIndex < verificationRandomCaseCount; caseIndex++ {
		integerValue := PortInt(random.Intn(2001) - 1000)
		floatValue := PortFloat(random.Intn(200001)-100000) / 100
		stringValue := PortString(fmt.Sprintf("typed-variable-%d-%d", caseIndex, random.Intn(1_000_000)))
		boolValue := PortBool(random.Intn(2) == 0)
		arrayValue := randomVerificationPortArray(random, random.Intn(9))
		input := fmt.Sprintf("integer=%d,float=%g,string=%q,bool=%t,array=%s", integerValue, floatValue, stringValue, boolValue, formatPortArray(arrayValue))
		assertVerificationRandomInputUnique(t, seen, "15", seed, caseIndex, input)
		assertVerificationRepeated(t, "15", seed, caseIndex, input, func() (PortArray, error) {
			return NewGraph(function).Do(FunctionEntranceID, integerValue, floatValue, stringValue, boolValue, arrayValue)
		}, referenceVariableTypes(integerValue, floatValue, stringValue, boolValue, arrayValue))
	}
}

func referenceVariableTypes(integerValue PortInt, floatValue PortFloat, stringValue PortString, boolValue PortBool, arrayValue PortArray) PortArray {
	return PortArray{
		{IntVal: integerValue},
		{FloatVal: floatValue},
		{StrVal: stringValue},
		{BoolVal: boolValue},
		{IntVal: PortInt(len(arrayValue))},
	}
}

func assertVerificationRandomInputUnique(t *testing.T, seen map[string]struct{}, asset string, seed int64, caseIndex int, input string) {
	t.Helper()
	if _, exists := seen[input]; exists {
		t.Fatalf("asset=%s seed=%d case=%d generated duplicate random input: %s", asset, seed, caseIndex, input)
	}
	seen[input] = struct{}{}
}

func assertVerificationRepeated(t *testing.T, asset string, seed int64, caseIndex int, input string, run func() (PortArray, error), want PortArray) {
	t.Helper()
	for repeat := 0; repeat < verificationRepeatCount; repeat++ {
		got, err := run()
		if err != nil {
			t.Fatalf("asset=%s seed=%d case=%d repeat=%d input=%s: %v", asset, seed, caseIndex, repeat, input, err)
		}
		assertVerificationReturnsWithContext(t, got, want, asset, seed, caseIndex, repeat, input)
	}
}

func assertVerificationReturnsWithContext(t *testing.T, got, want PortArray, asset string, seed int64, caseIndex, repeat int, input string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("asset=%s seed=%d case=%d repeat=%d input=%s returns=%#v, want=%#v", asset, seed, caseIndex, repeat, input, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("asset=%s seed=%d case=%d repeat=%d input=%s returns[%d]=%#v, want=%#v; all=%#v", asset, seed, caseIndex, repeat, input, index, got[index], want[index], got)
		}
	}
}

func randomVerificationIntInput(random *rand.Rand) [3]PortInt {
	return [3]PortInt{
		PortInt(random.Intn(2001) - 1000),
		PortInt(random.Intn(2001) - 1000),
		PortInt(random.Intn(2001) - 1000),
	}
}

func randomVerificationPortArray(random *rand.Rand, length int) PortArray {
	values := make(PortArray, length)
	for index := range values {
		values[index] = ArrayData{IntVal: PortInt(random.Intn(2001) - 1000)}
	}
	return values
}

func referenceLegacyShowcaseInteger() PortArray {
	returns := make(PortArray, 0, 10)
	for outer := PortInt(0); outer < 3; outer++ {
		for _, value := range []PortInt{2, 4, 6} {
			returns = append(returns, ArrayData{IntVal: outer + value})
		}
	}
	return append(returns, ArrayData{StrVal: "legacy-switch-hit"})
}
