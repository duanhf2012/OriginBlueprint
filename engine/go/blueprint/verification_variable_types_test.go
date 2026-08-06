package blueprint

import "testing"

func TestVerificationVariableTypesFunctionPreservesTypedValues(t *testing.T) {
	function := verificationFixtureFunction(t, loadVerificationFixtureSet(t), "functions/15_variable_types_lifecycle.obpf")
	items := PortArray{{IntVal: 3}, {IntVal: -5}, {IntVal: 8}}
	returns, err := NewGraph(function).Do(
		FunctionEntranceID,
		PortInt(-17),
		PortFloat(3.25),
		PortString("变量类型"),
		PortBool(true),
		items,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertVerificationReturns(t, returns, PortArray{
		{IntVal: -17},
		{FloatVal: 3.25},
		{StrVal: "变量类型"},
		{BoolVal: true},
		{IntVal: 3},
	})
}
