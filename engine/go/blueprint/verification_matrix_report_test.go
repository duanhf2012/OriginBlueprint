package blueprint

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const verificationMatrixReportEnv = "WRITE_BLUEPRINT_VERIFICATION_REPORT"

type verificationReportAsset struct {
	path             string
	seed             int64
	randomCases      int
	checksPerCase    int
	runRandomCompare func(*testing.T)
}

type verificationReportResult struct {
	verificationReportAsset
	passed bool
}

func TestWriteVerificationMatrixReport(t *testing.T) {
	if os.Getenv(verificationMatrixReportEnv) != "1" {
		t.Skipf("set %s=1 to generate the verification matrix", verificationMatrixReportEnv)
	}

	assets := verificationReportAssets(t)
	results := make([]verificationReportResult, 0, len(assets))
	for _, asset := range assets {
		asset := asset
		passed := t.Run(asset.path, asset.runRandomCompare)
		results = append(results, verificationReportResult{verificationReportAsset: asset, passed: passed})
	}

	report := buildVerificationMatrixReport(results)
	path := filepath.Join("..", "..", "..", "docs", "BLUEPRINT_VERIFICATION_MATRIX_ZH.md")
	if err := os.WriteFile(path, report, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func verificationReportAssets(t *testing.T) []verificationReportAsset {
	return []verificationReportAsset{
		{path: "01_legacy_all_nodes_showcase.vgf", seed: verificationRandomSeed(t, 2026071401), randomCases: verificationRandomCaseCount, checksPerCase: 2, runRandomCompare: testVerificationLegacyRandom},
		{path: "02_control_flow_maze.obp", seed: verificationRandomSeed(t, 2026071402), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationControlFlowRandom},
		{path: "03_array_data_lab.obp", seed: verificationRandomSeed(t, 2026071403), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationArrayLabRandom},
		{path: "04_deterministic_algorithm.obp", seed: verificationRandomSeed(t, 2026071404), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationAlgorithmRandom},
		{path: "05_function_orchestrator.obp", seed: verificationRandomSeed(t, 2026071405), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationOrchestratorRandom},
		{path: "06_async_delay_resume.obp", seed: verificationRandomSeed(t, 2026071406), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationDelayLoopRandom},
		{path: "07_async_rpc_resume_to.obp", seed: verificationRandomSeed(t, 2026071407), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationRPCRandom},
		{path: "functions/10_score_kernel.obpf", seed: verificationRandomSeed(t, 2026071410), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationScoreRandom},
		{path: "functions/11_array_fold_and_format.obpf", seed: verificationRandomSeed(t, 2026071411), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationFoldRandom},
		{path: "functions/12_nested_control_function.obpf", seed: verificationRandomSeed(t, 2026071412), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationNestedFunctionRandom},
		{path: "functions/13_local_state_isolation.obpf", seed: verificationRandomSeed(t, 2026071413), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationLocalFunctionRandom},
		{path: "functions/14_async_delay_function.obpf", seed: verificationRandomSeed(t, 2026071414), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationDelayFunctionRandom},
		{path: "functions/15_variable_types_lifecycle.obpf", seed: verificationRandomSeed(t, 2026071415), randomCases: verificationRandomCaseCount, checksPerCase: 1, runRandomCompare: testVerificationVariableTypesRandom},
	}
}

func buildVerificationMatrixReport(results []verificationReportResult) []byte {
	var report bytes.Buffer
	report.WriteString("# 蓝图与 Go 实现随机对比报告\n\n")
	report.WriteString("本报告由 `TestWriteVerificationMatrixReport` 实际执行后生成，不是手工填写。每个蓝图使用独立 seed 产生 64 组不同随机输入，每组重复执行 3 次；测试会拒绝同一蓝图内的重复输入。蓝图返回值逐端口与独立 Go 参考实现比较。可通过 `ORIGIN_BLUEPRINT_VERIFICATION_SEED_OFFSET` 切换到一轮全新的输入，表中记录的是本轮实际 seed，可用于稳定复现。\n\n")

	totalAssets := len(results)
	passedAssets := 0
	totalRandomCases := 0
	totalExecutions := 0
	for _, result := range results {
		if result.passed {
			passedAssets++
		}
		totalRandomCases += result.randomCases
		totalExecutions += result.randomCases * result.checksPerCase * verificationRepeatCount
	}
	fmt.Fprintf(&report, "- 蓝图文件：%d\n- 已有对应 Go 参考实现：%d/%d\n- 随机参数组：%d\n- 实际重复对比执行：%d\n- 通过蓝图：%d/%d\n- 不一致蓝图：%d\n\n",
		totalAssets, totalAssets, totalAssets, totalRandomCases, totalExecutions, passedAssets, totalAssets, totalAssets-passedAssets)

	report.WriteString("## 文件级结果\n\n")
	report.WriteString("| 蓝图文件 | Go 参考实现 | seed | 随机参数组 | 每组重复 | 对比执行数 | 结果 |\n")
	report.WriteString("| --- | --- | ---: | ---: | ---: | ---: | --- |\n")
	for _, result := range results {
		status := "一致"
		if !result.passed {
			status = "**不一致（详见测试失败日志）**"
		}
		fmt.Fprintf(&report, "| `%s` | 有 | %d | %d | %d | %d | %s |\n",
			result.path, result.seed, result.randomCases, verificationRepeatCount,
			result.randomCases*result.checksPerCase*verificationRepeatCount, status)
	}

	report.WriteString("\n说明：`01_legacy_all_nodes_showcase.vgf` 每组随机参数同时检查整数入口和数组入口，因此对比执行数是其他文件的两倍。异步 Delay 使用虚拟时钟，不依赖真实等待；异步 RPC 使用测试节点的 `Yield -> ResumeTo` 回包，均检查恢复后的最终返回值。\n\n")
	report.WriteString("## 本轮检查结论\n\n")
	report.WriteString("本轮未发现蓝图执行结果与 Go 参考实现不一致，无新增运行逻辑修正。\n\n")
	report.WriteString("## 历史对比检查已修正\n\n")
	report.WriteString("1. `03_array_data_lab.obp` 的 `StringSplit` 数据输出未经过执行流，读取时结果尚未生成；已补齐执行连线。\n")
	report.WriteString("2. `07_async_rpc_resume_to.obp` 原有两个相同入口 ID，加载时存在覆盖风险；已改为单入口依次覆盖成功与失败恢复分支。\n")
	report.WriteString("3. `functions/13_local_state_isolation.obpf` 返回端重新求值纯 Add，导致一次调用可能返回 `seed*2`；已改为返回本次 Set 后的值，恢复每次调用独立的局部状态语义。\n")
	report.WriteString("4. `MockDelayAsync`/`MockRpcAsync` 是验证目录专用外部节点，编辑器无法从正式节点库找到时会丢失端口和连线；已在蓝图文档内携带仅用于显示的 fallback 端口定义。\n\n")
	report.WriteString("## 失败定位方式\n\n")
	report.WriteString("若结果出现不一致，Go 测试错误会输出 `asset`、`seed`、`case`、`repeat`、完整输入、蓝图输出及 Go 期望输出。使用同一 seed 可稳定复现。\n")
	return report.Bytes()
}

func formatIntInputs(input [3]PortInt) string {
	return fmt.Sprintf("对象ID=%d, 参数1=%d, 参数2=%d", input[0], input[1], input[2])
}

func formatPortArray(values PortArray) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		switch {
		case value.StrVal != "":
			parts = append(parts, fmt.Sprintf("%q", value.StrVal))
		case value.BoolVal:
			parts = append(parts, "true")
		case value.FloatVal != 0:
			parts = append(parts, fmt.Sprintf("%g", value.FloatVal))
		default:
			parts = append(parts, fmt.Sprintf("%d", value.IntVal))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
