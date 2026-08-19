package blueprint

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAsPortArrayNumericString 覆盖蓝图数组端口填入字符串形式数字的解析场景。
// 用户反馈：数组类型参数填数字（字符串形式）时解析失败，业务读 IntVal 得到 0。
func TestAsPortArrayNumericString(t *testing.T) {
	// JSON `["34","34"]` 经 encoding/json 解码为 []any{"34","34"}。
	arr, ok := asPortArray([]any{"34", "34"})
	if !ok {
		t.Fatalf("asPortArray([]any{\"34\",\"34\"}) 解析失败")
	}
	if len(arr) != 2 {
		t.Fatalf("数组长度期望 2, 实际 %d", len(arr))
	}
	for i, item := range arr {
		if item.IntVal != 34 {
			t.Errorf("元素[%d] IntVal 期望 34, 实际 %d", i, item.IntVal)
		}
		if item.StrVal != "34" {
			t.Errorf("元素[%d] StrVal 期望 \"34\", 实际 %q", i, item.StrVal)
		}
	}
}

// TestAsPortArrayNonNumericString 非数字字符串只填 StrVal，IntVal 保持默认 0。
func TestAsPortArrayNonNumericString(t *testing.T) {
	arr, ok := asPortArray([]any{"abc"})
	if !ok {
		t.Fatalf("asPortArray([]any{\"abc\"}) 解析失败")
	}
	if len(arr) != 1 {
		t.Fatalf("数组长度期望 1, 实际 %d", len(arr))
	}
	if arr[0].StrVal != "abc" {
		t.Errorf("StrVal 期望 \"abc\", 实际 %q", arr[0].StrVal)
	}
	if arr[0].IntVal != 0 {
		t.Errorf("非数字字符串 IntVal 期望 0, 实际 %d", arr[0].IntVal)
	}
}

// TestAsPortArrayStringSlice Go 调用方传入 []string 走 []string 分支。
func TestAsPortArrayStringSlice(t *testing.T) {
	arr, ok := asPortArray([]string{"34", "abc"})
	if !ok {
		t.Fatalf("asPortArray([]string) 解析失败")
	}
	if len(arr) != 2 {
		t.Fatalf("数组长度期望 2, 实际 %d", len(arr))
	}
	if arr[0].IntVal != 34 || arr[0].StrVal != "34" {
		t.Errorf("元素0 期望 {IntVal:34, StrVal:\"34\"}, 实际 %+v", arr[0])
	}
	if arr[1].IntVal != 0 || arr[1].StrVal != "abc" {
		t.Errorf("元素1 期望 {IntVal:0, StrVal:\"abc\"}, 实际 %+v", arr[1])
	}
}

// TestAsPortArrayMixed 混合类型元素各自落到正确字段。
func TestAsPortArrayMixed(t *testing.T) {
	arr, ok := asPortArray([]any{"34", "abc", true})
	if !ok {
		t.Fatalf("asPortArray([]any mixed) 解析失败")
	}
	if len(arr) != 3 {
		t.Fatalf("数组长度期望 3, 实际 %d", len(arr))
	}
	if arr[0].IntVal != 34 || arr[0].StrVal != "34" {
		t.Errorf("元素0 期望数字双填, 实际 %+v", arr[0])
	}
	if arr[1].IntVal != 0 || arr[1].StrVal != "abc" {
		t.Errorf("元素1 期望只填 StrVal, 实际 %+v", arr[1])
	}
	if !arr[2].BoolVal {
		t.Errorf("元素2 期望 BoolVal=true, 实际 %+v", arr[2])
	}
}

// TestAsPortArrayNativeNumber 原生 JSON 数字（float64）路径保持不变。
func TestAsPortArrayNativeNumber(t *testing.T) {
	arr, ok := asPortArray([]any{2.0, 10.0})
	if !ok {
		t.Fatalf("asPortArray([]any{2.0,10.0}) 解析失败")
	}
	if len(arr) != 2 {
		t.Fatalf("数组长度期望 2, 实际 %d", len(arr))
	}
	if arr[0].IntVal != 2 {
		t.Errorf("元素0 IntVal 期望 2, 实际 %d", arr[0].IntVal)
	}
	if arr[1].IntVal != 10 {
		t.Errorf("元素1 IntVal 期望 10, 实际 %d", arr[1].IntVal)
	}
}

func TestAsPortIntRejectsFractionalAndOverflowingNumbers(t *testing.T) {
	if value, ok := asPortInt(float64(2)); !ok || value != 2 {
		t.Fatalf("integral float = %d,%v, want 2,true", value, ok)
	}
	if _, ok := asPortInt(float64(2.5)); ok {
		t.Fatal("fractional float must not be silently truncated to an integer")
	}
	if _, ok := asPortInt(uint64(1) << 63); ok {
		t.Fatal("uint64 overflow must not wrap into an integer")
	}
}

func TestAsPortArrayPreservesJSONInt64(t *testing.T) {
	decoder := json.NewDecoder(strings.NewReader(`[9223372036854775807,"-9223372036854775808"]`))
	decoder.UseNumber()
	var source []any
	if err := decoder.Decode(&source); err != nil {
		t.Fatal(err)
	}
	array, ok := asPortArray(source)
	if !ok || len(array) != 2 {
		t.Fatalf("array = %#v,%v", array, ok)
	}
	if array[0].IntVal != 9223372036854775807 || array[1].IntVal != -9223372036854775808 {
		t.Fatalf("int64 values changed: %#v", array)
	}
}

func TestArrayAssignmentRejectsUnsupportedElementsWithoutPartialMutation(t *testing.T) {
	port := NewPortArray()
	if err := port.setAnyValue([]any{1, "kept"}); err != nil {
		t.Fatal(err)
	}
	err := port.setAnyValue([]any{2, map[string]any{"nested": true}, 3})
	if err == nil || !strings.Contains(err.Error(), "element 1") {
		t.Fatalf("error = %v, want unsupported element index", err)
	}
	array, ok := port.GetArray()
	if !ok || len(array) != 2 || array[0].IntVal != 1 || array[1].StrVal != "kept" {
		t.Fatalf("failed assignment mutated port: %#v,%v", array, ok)
	}
}

func TestAsPortArrayPreservesFractionalJSONNumbersAsFloat(t *testing.T) {
	arr, ok := asPortArray([]any{2.5})
	if !ok || len(arr) != 1 {
		t.Fatalf("array = %#v,%v, want one element", arr, ok)
	}
	if arr[0].FloatVal != 2.5 || arr[0].IntVal != 0 {
		t.Fatalf("array element = %#v, want FloatVal 2.5 without integer truncation", arr[0])
	}
}

func TestPortArrayTracksMixedScalarTypesWithoutRestrictingStorage(t *testing.T) {
	port := NewPortArray()
	if err := port.setAnyValue([]any{float64(7), 2.5, false, ""}); err != nil {
		t.Fatalf("set mixed array: %v", err)
	}
	array, ok := port.GetArray()
	if !ok || len(array) != 4 {
		t.Fatalf("array = %#v,%v, want four mixed elements", array, ok)
	}
	if value, ok := port.GetArrayValInt(0); !ok || value != 7 {
		t.Fatalf("integer element = %d,%v, want 7,true", value, ok)
	}
	if _, ok := port.GetArrayValInt(1); ok {
		t.Fatal("Float element must not be readable as Integer")
	}
	if _, ok := port.GetArrayValInt(2); ok {
		t.Fatal("false Boolean element must retain its type")
	}
	if value, ok := port.GetArrayValStr(3); !ok || value != "" {
		t.Fatalf("empty String element = %q,%v, want empty,true", value, ok)
	}
}

func TestPortArrayKeepsNumericStringAndLegacyCompatibility(t *testing.T) {
	typed := NewPortArray()
	if err := typed.setAnyValue([]any{"0"}); err != nil {
		t.Fatalf("set numeric string: %v", err)
	}
	if value, ok := typed.GetArrayValInt(0); !ok || value != 0 {
		t.Fatalf("numeric string Integer compatibility = %d,%v", value, ok)
	}
	if value, ok := typed.GetArrayValStr(0); !ok || value != "0" {
		t.Fatalf("numeric string String compatibility = %q,%v", value, ok)
	}

	legacy := NewPortArray()
	if err := legacy.setAnyValue(PortArray{{}}); err != nil {
		t.Fatalf("set legacy zero element: %v", err)
	}
	if _, ok := legacy.GetArrayValInt(0); !ok {
		t.Fatal("untyped legacy zero element must keep field-based Integer access")
	}
	if _, ok := legacy.GetArrayValStr(0); !ok {
		t.Fatal("untyped legacy zero element must keep field-based String access")
	}
}

func TestPortArrayTypeMetadataSurvivesAnyBindingWithoutLeakingWrapper(t *testing.T) {
	source := NewPortArray()
	if err := source.setAnyValue([]any{2.5}); err != nil {
		t.Fatalf("set Float array: %v", err)
	}
	anyPort := NewPortAny()
	if err := assignPortValue(anyPort, source); err != nil {
		t.Fatalf("Array -> Any: %v", err)
	}
	if _, ok := anyPort.GetAny().(PortArray); !ok {
		t.Fatalf("public Any value type = %T, want PortArray", anyPort.GetAny())
	}
	target := NewPortArray()
	if err := assignPortValue(target, anyPort); err != nil {
		t.Fatalf("Any -> Array: %v", err)
	}
	if _, ok := target.GetArrayValInt(0); ok {
		t.Fatal("Float element type must survive Array -> Any -> Array binding")
	}
}
