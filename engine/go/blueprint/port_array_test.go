package blueprint

import "testing"

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
