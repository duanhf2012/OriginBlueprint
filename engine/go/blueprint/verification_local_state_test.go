package blueprint

import "testing"

func TestVerificationLocalStateFunctionStartsFromDefaultOnEveryCall(t *testing.T) {
	graphs := loadVerificationFixtureSet(t)
	function := verificationFixtureFunction(t, graphs, "functions/13_local_state_isolation.obpf")
	for call := 0; call < 2; call++ {
		returns, err := NewGraph(function).Do(FunctionEntranceID, PortInt(7))
		if err != nil {
			t.Fatalf("call %d: %v", call, err)
		}
		assertVerificationReturns(t, returns, PortArray{{IntVal: 7}})
	}
}
