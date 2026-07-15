param(
    [string]$OutputRoot = (Join-Path (Split-Path -Parent $PSScriptRoot) 'examples/verification-blueprints')
)

# Windows PowerShell 5 may decode UTF-8 scripts without a BOM using the system
# code page. Run generate-verification-blueprints.cmd to guarantee UTF-8 input.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)

function Node {
    param([string]$Id, [string]$TypeId, [int]$X, [int]$Y, [hashtable]$Values = @{}, [hashtable]$Properties = @{})
    return [ordered]@{
        id = $Id
        typeId = $TypeId
        position = [ordered]@{ x = $X; y = $Y }
        values = $Values
        properties = $Properties
    }
}

function Link {
    param([string]$Source, [string]$SourceOutput, [string]$Target, [string]$TargetInput)
    return [ordered]@{ source = $Source; sourceOutput = $SourceOutput; target = $Target; targetInput = $TargetInput }
}

function New-GraphGroup {
    param([string]$Id, [string]$Title, [int]$X, [int]$Y, [int]$Width, [int]$Height, [string[]]$NodeIds)
    return [ordered]@{ id = $Id; title = $Title; x = $X; y = $Y; width = $Width; height = $Height; nodeIds = $NodeIds }
}

function Port {
    param([string]$Id, [string]$Name, [string]$Type)
    return [ordered]@{ id = $Id; name = $Name; type = $Type }
}

function FunctionProperties {
    param([string]$Role, [string]$FunctionId, [string]$FunctionName, [hashtable]$Signature)
    return [ordered]@{
        label = "$FunctionName $Role"
        functionRole = $Role.ToLowerInvariant()
        functionId = $FunctionId
        functionName = $FunctionName
        functionSource = 'workspace'
        functionSignature = $Signature
    }
}

function FunctionCallProperties {
    param([string]$FunctionId, [string]$FunctionName, [hashtable]$Signature)
    return [ordered]@{
        label = "Call $FunctionName"
        functionId = $FunctionId
        functionName = $FunctionName
        functionSource = 'workspace'
        functionSignature = $Signature
    }
}

function NativeDocument {
    param([string]$Name, [array]$Nodes, [array]$Connections, [array]$Groups = @(), [array]$Variables = @(), [hashtable]$Extra = @{})
    $document = [ordered]@{
        schemaVersion = 1
        graphName = $Name
        nodes = $Nodes
        connections = $Connections
        groups = $Groups
        variables = $Variables
        variableGroups = @([ordered]@{ id = 'default'; name = '默认'; collapsed = $false }, [ordered]@{ id = 'local'; name = '局部状态'; collapsed = $false })
        view = [ordered]@{ x = 0; y = 0; zoom = 0.72 }
    }
    foreach ($key in $Extra.Keys) { $document[$key] = $Extra[$key] }
    return $document
}

function WriteArtifact {
    param([string]$RelativePath, $Value)
    $path = Join-Path $OutputRoot $RelativePath
    $directory = Split-Path -Parent $path
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
    if ($Value -is [string]) {
        [System.IO.File]::WriteAllText($path, $Value, $Utf8NoBom)
        return
    }
    $content = $Value | ConvertTo-Json -Depth 32
    [System.IO.File]::WriteAllText($path, $content, $Utf8NoBom)
}

function LegacyNode {
    param([string]$Id, [string]$Class, [int]$X, [int]$Y, [hashtable]$Defaults = @{})
    return [ordered]@{ id = $Id; class = $Class; module = 'verification.fixture'; pos = @($X, $Y); port_defaultv = $Defaults }
}

function LegacyEdge {
    param([string]$Source, [int]$SourcePort, [string]$Target, [int]$TargetPort)
    return [ordered]@{ source_node_id = $Source; source_port_id = $SourcePort; des_node_id = $Target; des_port_id = $TargetPort }
}

Remove-Item -LiteralPath $OutputRoot -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path (Join-Path $OutputRoot 'functions') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $OutputRoot 'nodes') | Out-Null

# 仅供验证样本使用的异步业务节点定义。正式系统节点仍只位于仓库根目录 nodes/。
$mockRpcAsyncSchema = [ordered]@{
    id = 'origin.example.mock-rpc-async'
    sourceName = 'MockRpcAsync'
    title = '模拟 RPC 异步调用'
    titleEn = 'Mock RPC Async Call'
    category = '示例 / 异步'
    categoryEn = 'Examples / Async'
    subtitle = '定时器到期后选择成功或失败出口，演示 ResumeTo。'
    subtitleEn = 'Uses a timer to choose the success or failure ResumeTo branch.'
    width = 330
    inputs = @(
        [ordered]@{ key = 'exec'; label = '执行'; labelEn = 'Exec'; type = 'exec' },
        [ordered]@{ key = 'delayMs'; label = '延迟（毫秒）'; labelEn = 'Delay (ms)'; type = 'data'; data_type = 'Integer'; defaultValue = 50 },
        [ordered]@{ key = 'succeed'; label = '是否成功'; labelEn = 'Succeed'; type = 'data'; data_type = 'Boolean'; defaultValue = $true },
        [ordered]@{ key = 'successValue'; label = '成功值'; labelEn = 'Success Value'; type = 'data'; data_type = 'Integer'; defaultValue = 100 },
        [ordered]@{ key = 'failureCode'; label = '失败码'; labelEn = 'Failure Code'; type = 'data'; data_type = 'Integer'; defaultValue = 500 },
        [ordered]@{ key = 'failureMessage'; label = '失败信息'; labelEn = 'Failure Message'; type = 'data'; data_type = 'String'; defaultValue = 'mock rpc failed' }
    )
    outputs = @(
        [ordered]@{ key = 'succeeded'; label = '成功'; labelEn = 'Succeeded'; type = 'exec' },
        [ordered]@{ key = 'failed'; label = '失败'; labelEn = 'Failed'; type = 'exec' },
        [ordered]@{ key = 'value'; label = '成功值'; labelEn = 'Value'; type = 'data'; data_type = 'Integer' },
        [ordered]@{ key = 'errorCode'; label = '失败码'; labelEn = 'Error Code'; type = 'data'; data_type = 'Integer' },
        [ordered]@{ key = 'errorMessage'; label = '失败信息'; labelEn = 'Error Message'; type = 'data'; data_type = 'String' }
    )
}
WriteArtifact 'nodes/MockRpcAsync.json' $mockRpcAsyncSchema
$mockRpcFallbackProperties = @{
    label = '模拟 RPC 异步调用'
    legacyClass = 'MockRpcAsync'
    legacyModule = 'verification.fixture'
    legacyInputs = @(
        [ordered]@{ key = 'exec'; label = '执行'; type = 'exec' },
        [ordered]@{ key = 'delayMs'; label = '延迟（毫秒）'; type = 'integer' },
        [ordered]@{ key = 'succeed'; label = '是否成功'; type = 'boolean' },
        [ordered]@{ key = 'successValue'; label = '成功值'; type = 'integer' },
        [ordered]@{ key = 'failureCode'; label = '失败码'; type = 'integer' },
        [ordered]@{ key = 'failureMessage'; label = '失败信息'; type = 'string' }
    )
    legacyOutputs = @(
        [ordered]@{ key = 'succeeded'; label = '成功'; type = 'exec' },
        [ordered]@{ key = 'failed'; label = '失败'; type = 'exec' },
        [ordered]@{ key = 'value'; label = '成功值'; type = 'integer' },
        [ordered]@{ key = 'errorCode'; label = '失败码'; type = 'integer' },
        [ordered]@{ key = 'errorMessage'; label = '失败信息'; type = 'string' }
    )
}

$mockDelayAsyncSchema = [ordered]@{
    id = 'origin.example.mock-delay-async'
    sourceName = 'MockDelayAsync'
    title = '模拟 Delay 异步恢复'
    titleEn = 'Mock Delay Async Resume'
    category = '示例 / 异步'
    categoryEn = 'Examples / Async'
    subtitle = '测试专用：挂起当前 Execution，并在假时钟到期后从原位置恢复。'
    subtitleEn = 'Test only: yields the current execution and resumes it after the fake deadline.'
    width = 330
    inputs = @(
        [ordered]@{ key = 'exec'; label = '执行'; labelEn = 'Exec'; type = 'exec' },
        [ordered]@{ key = 'delayMs'; label = '延迟（毫秒）'; labelEn = 'Delay (ms)'; type = 'data'; data_type = 'Integer'; defaultValue = 10 },
        [ordered]@{ key = 'value'; label = '透传整数'; labelEn = 'Pass-through Value'; type = 'data'; data_type = 'Integer'; defaultValue = 0 },
        [ordered]@{ key = 'tag'; label = '透传标记'; labelEn = 'Pass-through Tag'; type = 'data'; data_type = 'String'; defaultValue = 'delay-resumed' }
    )
    outputs = @(
        [ordered]@{ key = 'completed'; label = '完成'; labelEn = 'Completed'; type = 'exec' },
        [ordered]@{ key = 'value'; label = '恢复整数'; labelEn = 'Resumed Value'; type = 'data'; data_type = 'Integer' },
        [ordered]@{ key = 'tag'; label = '恢复标记'; labelEn = 'Resumed Tag'; type = 'data'; data_type = 'String' }
    )
}
WriteArtifact 'nodes/MockDelayAsync.json' $mockDelayAsyncSchema
$mockDelayFallbackProperties = @{
    label = '模拟 Delay 异步恢复'
    legacyClass = 'MockDelayAsync'
    legacyModule = 'verification.fixture'
    legacyInputs = @(
        [ordered]@{ key = 'exec'; label = '执行'; type = 'exec' },
        [ordered]@{ key = 'delayMs'; label = '延迟（毫秒）'; type = 'integer' },
        [ordered]@{ key = 'value'; label = '透传整数'; type = 'integer' },
        [ordered]@{ key = 'tag'; label = '透传标记'; type = 'string' }
    )
    legacyOutputs = @(
        [ordered]@{ key = 'completed'; label = '完成'; type = 'exec' },
        [ordered]@{ key = 'value'; label = '恢复整数'; type = 'integer' },
        [ordered]@{ key = 'tag'; label = '恢复标记'; type = 'string' }
    )
}

$scoreSignature = [ordered]@{
    inputs = @(
        (Port 'base' '基础分' 'integer'),
        (Port 'bonus' '附加分' 'integer'),
        (Port 'multiplier' '倍率' 'integer')
    )
    outputs = @(
        (Port 'score' '评分' 'integer'),
        (Port 'tier' '等级' 'string')
    )
}
$arraySignature = [ordered]@{
    inputs = @(
        (Port 'items' '整数数组' 'array'),
        (Port 'weight' '权重' 'integer')
    )
    outputs = @(
        (Port 'sum' '累计值' 'integer'),
        (Port 'summary' '摘要' 'string')
    )
}
$controlSignature = [ordered]@{
    inputs = @(
        (Port 'start' '起始值' 'integer'),
        (Port 'limit' '上限' 'integer')
    )
    outputs = @(
        (Port 'count' '次数' 'integer'),
        (Port 'trace' '轨迹' 'string')
    )
}
$localSignature = [ordered]@{
    inputs = @((Port 'seed' '种子' 'integer'))
    outputs = @((Port 'result' '局部结果' 'integer'))
}
$asyncDelaySignature = [ordered]@{
    inputs = @(
        (Port 'delayMs' '延迟毫秒' 'integer'),
        (Port 'value' '透传整数' 'integer'),
        (Port 'tag' '透传标记' 'string')
    )
    outputs = @(
        (Port 'value' '恢复整数' 'integer'),
        (Port 'tag' '恢复标记' 'string')
    )
}

# 01: legacy 格式。两个入口分别形成整数与数组控制流。
$legacyNodes = @(
    (LegacyNode 'entry_int' 'Entrance_IntParam_000001' 80 120),
    (LegacyNode 'sequence' 'Sequence' 310 120),
    (LegacyNode 'for_loop' 'Foreach' 530 120 @{ '1' = 0; '2' = 3 }),
    (LegacyNode 'int_array' 'CreateIntArray' 520 300 @{ '0' = @(2, 4, 6) }),
    (LegacyNode 'array_loop' 'ForeachIntArray' 760 120),
    (LegacyNode 'add' 'AddInt' 980 120),
    (LegacyNode 'append' 'AppendIntReturn' 1190 120),
    (LegacyNode 'range' 'RangeCompare' 520 510 @{ '1' = 4; '2' = @(3, 6, 9) }),
    (LegacyNode 'switch' 'EqualSwitch' 760 510 @{ '1' = 7; '2' = @(1, 7, 9) }),
    (LegacyNode 'append_text' 'AppendStringReturn' 1000 510 @{ '1' = 'legacy-switch-hit' }),
    (LegacyNode 'entry_array' 'Entrance_ArrayParam_000002' 80 760),
    (LegacyNode 'debug' 'DebugOutput' 320 760 @{ '1' = 9; '2' = 'legacy showcase'; '3' = @(1, 2) }),
    (LegacyNode 'sub' 'SubInt' 520 760 @{ '0' = 11; '1' = 4; '2' = $false }),
    (LegacyNode 'mul' 'MulInt' 720 760 @{ '0' = 3; '1' = 7 }),
    (LegacyNode 'div' 'DivInt' 920 760 @{ '0' = 20; '1' = 3; '2' = $true }),
    (LegacyNode 'mod' 'ModInt' 1120 760 @{ '0' = 20; '1' = 6 }),
    (LegacyNode 'random' 'RandNumber' 1320 760 @{ '0' = 99; '1' = 42; '2' = 42 }),
    (LegacyNode 'bool_if' 'BoolIf' 520 930 @{ '1' = $true }),
    (LegacyNode 'greater' 'GreaterThanInteger' 720 930 @{ '1' = $false; '2' = 9; '3' = 4 }),
    (LegacyNode 'less' 'LessThanInteger' 920 930 @{ '1' = $true; '2' = 4; '3' = 4 }),
    (LegacyNode 'equal' 'EqualInteger' 1120 930 @{ '1' = 4; '2' = 4 }),
    (LegacyNode 'probability' 'Probability' 1320 930 @{ '1' = 10000 }),
    (LegacyNode 'timer_debug' 'DebugOutput' 1550 760 @{ '2' = 'legacy timer chain' })
)
$legacyEdges = @(
    (LegacyEdge 'entry_int' 0 'sequence' 0),
    (LegacyEdge 'sequence' 0 'for_loop' 0),
    (LegacyEdge 'for_loop' 0 'array_loop' 0),
    (LegacyEdge 'int_array' 0 'array_loop' 1),
    (LegacyEdge 'array_loop' 0 'append' 0),
    (LegacyEdge 'for_loop' 2 'add' 0),
    (LegacyEdge 'array_loop' 3 'add' 1),
    (LegacyEdge 'add' 0 'append' 1),
    (LegacyEdge 'sequence' 1 'range' 0),
    (LegacyEdge 'range' 3 'switch' 0),
    (LegacyEdge 'switch' 3 'append_text' 0),
    (LegacyEdge 'entry_array' 0 'debug' 0),
    (LegacyEdge 'entry_array' 1 'debug' 1),
    (LegacyEdge 'entry_array' 2 'debug' 3),
    (LegacyEdge 'debug' 0 'bool_if' 0),
    (LegacyEdge 'bool_if' 1 'greater' 0),
    (LegacyEdge 'greater' 1 'less' 0),
    (LegacyEdge 'less' 1 'equal' 0),
    (LegacyEdge 'equal' 1 'probability' 0),
    (LegacyEdge 'sub' 0 'mul' 0),
    (LegacyEdge 'mul' 0 'div' 0),
    (LegacyEdge 'div' 0 'mod' 0),
    (LegacyEdge 'mod' 0 'random' 0),
    (LegacyEdge 'probability' 1 'timer_debug' 0),
    (LegacyEdge 'random' 0 'timer_debug' 1)
)
WriteArtifact '01_legacy_all_nodes_showcase.vgf' ([ordered]@{ graph_name = 'Legacy All Nodes Showcase'; time = '2026-07-11T00:00:00Z'; nodes = $legacyNodes; edges = $legacyEdges; groups = @(); variables = @() })

# 02: 一条可追踪的控制流。所有节点均从同一入口可达并影响返回结果。
$controlNodes = @(
    (Node 'entry' 'origin.event.entry-two-integers' 80 130),
    (Node 'sequence' 'origin.flow.sequence' 290 130 @{} @{ label = '主流程序列'; dynamicOutputCount = 1 }),
    (Node 'outer_loop' 'origin.flow.for-loop' 520 90 @{ start = 0; end = 3 }),
    (Node 'numbers' 'origin.array.create-integer-new' 520 250 @{ items = @(2, 4, 6) }),
    (Node 'inner_loop' 'origin.flow.foreach-integer-array' 760 90),
    (Node 'sum' 'origin.math.add-integer' 980 90),
    (Node 'loop_return' 'origin.result.append-integer' 1200 90),
    (Node 'range' 'origin.flow.range-compare' 520 440 @{ value = 4; ranges = @(3, 6, 10) }),
    (Node 'branch' 'origin.flow.branch' 760 440 @{ condition = $true }),
    (Node 'range_return' 'origin.result.append-string' 1000 440 @{ value = 'range-branch-true' }),
    (Node 'break_loop' 'origin.flow.for-loop-break' 520 690 @{ start = 0; end = 5 }),
    (Node 'break_compare' 'origin.compare.greater-integer' 760 650 @{ b = 2 }),
    (Node 'break_branch' 'origin.flow.branch' 980 690),
    (Node 'break_value' 'origin.result.append-integer' 1200 650),
    (Node 'break_done' 'origin.result.append-string' 1200 760 @{ value = 'break-loop-complete' }),
    (Node 'probability' 'origin.flow.probability' 520 950 @{ probability = 10000 }),
    (Node 'probability_return' 'origin.result.append-string' 760 950 @{ value = 'probability-hit' }),
    (Node 'while' 'origin.flow.while' 1000 950 @{ condition = $false }),
    (Node 'greater' 'origin.flow.greater-integer' 520 1210 @{ orEqual = $false; a = 9; b = 4 }),
    (Node 'less' 'origin.flow.less-integer' 740 1210 @{ orEqual = $true; a = 4; b = 4 }),
    (Node 'equal' 'origin.flow.equal-integer' 960 1210 @{ a = 7; b = 7 }),
    (Node 'switch_legacy' 'origin.flow.equal-switch' 1180 1210 @{ value = 7; cases = @(1, 7, 9) }),
    (Node 'switch_new' 'origin.flow.equal-switch-new' 1440 1210 @{ value = 8; cases = @(2, 8, 10) }),
    (Node 'switch_return' 'origin.result.append-string' 1700 1210 @{ value = 'comparison-switch-complete' }),
    (Node 'string_items' 'origin.array.create-string-new' 520 1460 @{ items = @('alpha', 'beta', 'gamma') }),
    (Node 'foreach_any' 'origin.flow.foreach-array' 760 1460),
    (Node 'cast_any' 'origin.cast.any-string' 1010 1460),
    (Node 'any_return' 'origin.result.append-string' 1260 1460),
    (Node 'flow_done' 'origin.result.append-string' 1010 1620 @{ value = 'control-flow-complete' })
)
$controlLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'outer_loop' 'exec'),
    (Link 'outer_loop' 'body' 'inner_loop' 'exec'),
    (Link 'numbers' 'array' 'inner_loop' 'array'),
    (Link 'inner_loop' 'body' 'loop_return' 'exec'),
    (Link 'outer_loop' 'index' 'sum' 'a'),
    (Link 'inner_loop' 'value' 'sum' 'b'),
    (Link 'sum' 'result' 'loop_return' 'value'),
    (Link 'outer_loop' 'completed' 'range' 'exec'),
    (Link 'range' 'case2' 'branch' 'exec'),
    (Link 'branch' 'true' 'range_return' 'exec'),
    (Link 'range_return' 'exec' 'break_loop' 'exec'),
    (Link 'break_loop' 'body' 'break_branch' 'exec'),
    (Link 'break_loop' 'index' 'break_value' 'value'),
    (Link 'break_loop' 'index' 'break_compare' 'a'),
    (Link 'break_compare' 'result' 'break_branch' 'condition'),
    (Link 'break_branch' 'true' 'break_loop' 'break'),
    (Link 'break_branch' 'false' 'break_value' 'exec'),
    (Link 'break_loop' 'completed' 'break_done' 'exec'),
    (Link 'break_done' 'exec' 'probability' 'exec'),
    (Link 'probability' 'hit' 'probability_return' 'exec'),
    (Link 'probability_return' 'exec' 'while' 'exec'),
    (Link 'while' 'completed' 'greater' 'exec'),
    (Link 'greater' 'true' 'less' 'exec'),
    (Link 'less' 'true' 'equal' 'exec'),
    (Link 'equal' 'true' 'switch_legacy' 'exec'),
    (Link 'switch_legacy' 'case2' 'switch_new' 'exec'),
    (Link 'switch_new' 'case2' 'switch_return' 'exec'),
    (Link 'switch_return' 'exec' 'foreach_any' 'exec'),
    (Link 'string_items' 'array' 'foreach_any' 'array'),
    (Link 'foreach_any' 'body' 'cast_any' 'exec'),
    (Link 'foreach_any' 'value' 'cast_any' 'value'),
    (Link 'cast_any' 'exec' 'any_return' 'exec'),
    (Link 'cast_any' 'result' 'any_return' 'value'),
    (Link 'foreach_any' 'completed' 'flow_done' 'exec')
)
WriteArtifact '02_control_flow_maze.obp' (NativeDocument '控制流迷宫' $controlNodes $controlLinks @(
    (New-GraphGroup 'loop-flow' '入口、Sequence 与嵌套循环' 35 25 1420 350 @('entry','sequence','outer_loop','numbers','inner_loop','sum','loop_return')),
    (New-GraphGroup 'branch-flow' 'Range、Branch 与真实 Break' 455 380 1040 470 @('range','branch','range_return','break_loop','break_compare','break_branch','break_value','break_done')),
    (New-GraphGroup 'comparison-flow' '概率、While、比较、Switch 与任意数组遍历' 455 875 1550 830 @('probability','probability_return','while','greater','less','equal','switch_legacy','switch_new','switch_return','string_items','foreach_any','cast_any','any_return','flow_done'))
))

# 03: 数组、字符串、变量和转换。每个实验区使用局部入口，避免跨分组连线。
$arrayVariables = @(
    [ordered]@{ id = 'array_sum'; name = 'ArraySum'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '数组实验室中的累计值' },
    [ordered]@{ id = 'array_label'; name = 'ArrayLabel'; type = 'string'; defaultValue = 'cold'; groupId = 'local'; description = '局部字符串变量' }
)
$arrayNodes = @(
    (Node 'entry' 'origin.event.entry-array' 600 130),
    (Node 'integers_old' 'origin.array.create-integer' 300 80 @{ items = @(3, 1, 4, 1, 5) }),
    (Node 'get_integer' 'origin.array.get-integer' 540 80 @{ index = 2 }),
    (Node 'integer_return' 'origin.result.append-integer' 780 80),
    (Node 'integers_new' 'origin.array.create-integer-new' 300 240 @{ items = @(3, 1, 4, 1, 5) }),
    (Node 'append_integer' 'origin.array.append-integer' 540 240 @{ value = 9 }),
    (Node 'length' 'origin.array.length' 780 240),
    (Node 'length_return' 'origin.result.append-integer' 1020 240),
    (Node 'sum_set' 'origin.variable.set' 620 390 @{ value = 0 } @{ variableId = 'array_sum'; variableAccess = 'set'; label = 'Set ArraySum' }),
    (Node 'sum_get' 'origin.variable.get' 850 390 @{} @{ variableId = 'array_sum'; variableAccess = 'get'; label = 'Get ArraySum' }),
    (Node 'sum_return' 'origin.result.append-integer' 1020 390),
    (Node 'strings_old' 'origin.array.create-string' 300 600 @{ items = @('red', 'green', 'blue') }),
    (Node 'get_old_string' 'origin.array.get-string' 540 600 @{ index = 1 }),
    (Node 'old_string_return' 'origin.result.append-string' 780 600),
    (Node 'strings_new' 'origin.array.create-string-new' 300 760 @{ items = @('red', 'green', 'blue') }),
    (Node 'append_string' 'origin.array.append-string' 540 760 @{ value = 'violet' }),
    (Node 'get_string' 'origin.array.get-string' 780 760 @{ index = 3 }),
    (Node 'string_return' 'origin.result.append-string' 1020 760),
    (Node 'literal' 'origin.literal.string' 300 1180 @{ value = 'east|west|north' }),
    (Node 'split' 'origin.string.split' 540 1180 @{ delimiter = '|' }),
    (Node 'get_any' 'origin.array.get-any' 790 1180 @{ index = 2 }),
    (Node 'cast_any' 'origin.cast.any-string' 1040 1080),
    (Node 'label_set' 'origin.variable.set' 1280 1080 @{ value = 'array-lab' } @{ variableId = 'array_label'; variableAccess = 'set'; label = 'Set ArrayLabel' }),
    (Node 'label_get' 'origin.variable.get' 1280 1220 @{} @{ variableId = 'array_label'; variableAccess = 'get'; label = 'Get ArrayLabel' }),
    (Node 'any_return' 'origin.result.append-string' 1520 1080)
)
$arrayLinks = @(
    (Link 'entry' 'exec' 'integer_return' 'exec'),
    (Link 'integers_old' 'array' 'get_integer' 'array'),
    (Link 'get_integer' 'value' 'integer_return' 'value'),
    (Link 'integer_return' 'exec' 'length_return' 'exec'),
    (Link 'integers_new' 'array' 'append_integer' 'array'),
    (Link 'append_integer' 'array' 'length' 'array'),
    (Link 'length' 'length' 'length_return' 'value'),
    (Link 'length_return' 'exec' 'sum_set' 'exec'),
    (Link 'get_integer' 'value' 'sum_set' 'value'),
    (Link 'sum_set' 'exec' 'sum_return' 'exec'),
    (Link 'sum_get' 'value' 'sum_return' 'value'),
    (Link 'sum_return' 'exec' 'old_string_return' 'exec'),
    (Link 'strings_old' 'array' 'get_old_string' 'array'),
    (Link 'get_old_string' 'value' 'old_string_return' 'value'),
    (Link 'old_string_return' 'exec' 'string_return' 'exec'),
    (Link 'strings_new' 'array' 'append_string' 'array'),
    (Link 'append_string' 'array' 'get_string' 'array'),
    (Link 'get_string' 'value' 'string_return' 'value'),
    (Link 'string_return' 'exec' 'split' 'exec'),
    (Link 'literal' 'value' 'split' 'text'),
    (Link 'split' 'array' 'get_any' 'array'),
    (Link 'get_any' 'value' 'cast_any' 'value'),
    (Link 'split' 'exec' 'cast_any' 'exec'),
    (Link 'cast_any' 'exec' 'label_set' 'exec'),
    (Link 'cast_any' 'result' 'label_set' 'value'),
    (Link 'label_set' 'exec' 'any_return' 'exec'),
    (Link 'label_get' 'value' 'any_return' 'value')
)
WriteArtifact '03_array_data_lab.obp' (NativeDocument '数组数据实验室' $arrayNodes $arrayLinks @(
    (New-GraphGroup 'integer-arrays' '入口、整数数组、追加、读取、长度与局部变量' 35 25 1240 470 @('entry','integers_old','get_integer','integer_return','integers_new','append_integer','length','length_return','sum_set','sum_get','sum_return')),
    (New-GraphGroup 'string-arrays' '字符串数组：旧/新创建、追加和读取' 35 545 1240 310 @('strings_old','get_old_string','old_string_return','strings_new','append_string','get_string','string_return')),
    (New-GraphGroup 'text-arrays' '字符串切分、任意项读取和局部变量' 35 975 1740 360 @('literal','split','get_any','cast_any','any_return','label_set','label_get'))
) $arrayVariables)

# 04: 确定性算法。运算、浮点运算、比较和分支均使用固定值或固定入口参数。
$algorithmNodes = @(
    (Node 'entry' 'origin.event.entry-two-integers' 80 120),
    (Node 'sequence' 'origin.flow.sequence' 290 120 @{} @{ label = '评分算法序列'; dynamicOutputCount = 4 }),
    (Node 'add' 'origin.math.add-integer' 510 80),
    (Node 'subtract' 'origin.math.subtract-integer' 730 80 @{ b = 3; absolute = $false }),
    (Node 'multiply' 'origin.math.multiply-integer' 950 80 @{ b = 2 }),
    (Node 'divide' 'origin.math.divide-integer' 1170 80 @{ b = 3; round = $false }),
    (Node 'modulo' 'origin.math.modulo-integer' 1390 80 @{ b = 7 }),
    (Node 'random_fixed' 'origin.math.random-integer' 1610 80 @{ seed = 77; min = 42; max = 42 }),
    (Node 'score_return' 'origin.result.append-integer' 1840 80),
    (Node 'mod_return' 'origin.result.append-integer' 1840 230),
    (Node 'random_return' 'origin.result.append-integer' 1840 380),
    (Node 'compare_data' 'origin.compare.greater-integer' 510 500 @{ b = 10 }),
    (Node 'branch' 'origin.flow.branch' 760 500),
    (Node 'high_text' 'origin.result.append-string' 1010 450 @{ value = 'score-high' }),
    (Node 'low_text' 'origin.result.append-string' 1010 570 @{ value = 'score-low' }),
    (Node 'range' 'origin.flow.range-compare' 1260 500 @{ value = 12; ranges = @(5, 10, 20) }),
    (Node 'range_text' 'origin.result.append-string' 1510 500 @{ value = 'range-case-3' }),
    (Node 'switch' 'origin.flow.equal-switch-new' 1260 700 @{ value = 2; cases = @(1, 2, 3) }),
    (Node 'switch_text' 'origin.result.append-string' 1510 700 @{ value = 'switch-case-2' }),
    (Node 'add_float' 'origin.math.add-float' 510 920 @{ a = 1.5; b = 2.25 }),
    (Node 'sub_float' 'origin.math.subtract-float' 730 920 @{ a = 6.5; b = 1.25 }),
    (Node 'mul_float' 'origin.math.multiply-float' 950 920 @{ a = 2.5; b = 4.0 }),
    (Node 'div_float' 'origin.math.divide-float' 1170 920 @{ a = 9.0; b = 2.0 }),
    (Node 'float_cast' 'origin.cast.float-string' 1390 920),
    (Node 'float_return' 'origin.result.append-string' 1610 920),
    (Node 'integer_cast' 'origin.cast.integer-string' 1390 1050),
    (Node 'debug' 'origin.debug.output' 1610 1050 @{ integer = 42; string = 'deterministic-algorithm'; array = @(1, 2, 3) })
)
$algorithmLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'entry' 'param1' 'add' 'a'),
    (Link 'entry' 'param2' 'add' 'b'),
    (Link 'add' 'result' 'subtract' 'a'),
    (Link 'subtract' 'result' 'multiply' 'a'),
    (Link 'multiply' 'result' 'divide' 'a'),
    (Link 'divide' 'result' 'modulo' 'a'),
    (Link 'sequence' 'then0' 'score_return' 'exec'),
    (Link 'divide' 'result' 'score_return' 'value'),
    (Link 'score_return' 'exec' 'mod_return' 'exec'),
    (Link 'modulo' 'result' 'mod_return' 'value'),
    (Link 'mod_return' 'exec' 'random_return' 'exec'),
    (Link 'random_fixed' 'result' 'random_return' 'value'),
    (Link 'sequence' 'then1' 'branch' 'exec'),
    (Link 'divide' 'result' 'compare_data' 'a'),
    (Link 'compare_data' 'result' 'branch' 'condition'),
    (Link 'branch' 'true' 'high_text' 'exec'),
    (Link 'branch' 'false' 'low_text' 'exec'),
    (Link 'sequence' 'then2' 'range' 'exec'),
    (Link 'range' 'case3' 'range_text' 'exec'),
    (Link 'range_text' 'exec' 'switch' 'exec'),
    (Link 'switch' 'case2' 'switch_text' 'exec'),
    (Link 'add_float' 'result' 'sub_float' 'a'),
    (Link 'sub_float' 'result' 'mul_float' 'a'),
    (Link 'mul_float' 'result' 'div_float' 'a'),
    (Link 'div_float' 'result' 'float_cast' 'value'),
    (Link 'sequence' 'then3' 'float_return' 'exec'),
    (Link 'float_cast' 'result' 'float_return' 'value'),
    (Link 'float_return' 'exec' 'debug' 'exec'),
    (Link 'random_fixed' 'result' 'integer_cast' 'value'),
    (Link 'integer_cast' 'result' 'debug' 'string'),
    (Link 'random_fixed' 'result' 'debug' 'integer')
)
WriteArtifact '04_deterministic_algorithm.obp' (NativeDocument '确定性评分算法' $algorithmNodes $algorithmLinks @(
    (New-GraphGroup 'integer-algorithm' '整数评分、取模和固定随机数' 35 30 2020 430 @('entry','sequence','add','subtract','multiply','divide','modulo','random_fixed','score_return','mod_return','random_return')),
    (New-GraphGroup 'classification' '数据比较、Branch、Range 与 Switch 分类' 455 450 1250 340 @('compare_data','branch','high_text','low_text','range','range_text','switch','switch_text')),
    (New-GraphGroup 'float-gallery' '浮点运算、转换和调试输出' 455 870 1400 260 @('add_float','sub_float','mul_float','div_float','float_cast','float_return','integer_cast','debug'))
))

# 10: 函数评分核心，返回确定的整数评分和等级文本。
$scoreNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/10_score_kernel.obpf' '评分核心' $scoreSignature)),
    (Node 'sequence' 'origin.flow.sequence' 210 130 @{} @{ label = '评分函数序列'; dynamicOutputCount = 1 }),
    (Node 'add' 'origin.math.add-integer' 340 120),
    (Node 'multiply' 'origin.math.multiply-integer' 560 120),
    (Node 'range' 'origin.flow.range-compare' 780 120 @{ value = 12; ranges = @(5, 10, 20) }),
    (Node 'switch' 'origin.flow.equal-switch-new' 1010 120 @{ value = 2; cases = @(1, 2, 3) }),
    (Node 'probability' 'origin.flow.probability' 1240 120 @{ probability = 10000 }),
    (Node 'tier' 'origin.literal.string' 1020 350 @{ value = 'gold' }),
    (Node 'local_set' 'origin.variable.set' 560 430 @{ value = 0 } @{ variableId = 'score_local'; variableAccess = 'set'; label = 'Set ScoreLocal' }),
    (Node 'local_get' 'origin.variable.get' 790 430 @{} @{ variableId = 'score_local'; variableAccess = 'get'; label = 'Get ScoreLocal' }),
    (Node 'return' 'origin.function.return' 1500 130 @{} (FunctionProperties 'Return' 'functions/10_score_kernel.obpf' '评分核心' $scoreSignature))
)
$scoreLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'range' 'exec'),
    (Link 'entry' 'input_base' 'add' 'a'),
    (Link 'entry' 'input_bonus' 'add' 'b'),
    (Link 'add' 'result' 'multiply' 'a'),
    (Link 'entry' 'input_multiplier' 'multiply' 'b'),
    (Link 'multiply' 'result' 'local_set' 'value'),
    (Link 'range' 'case3' 'switch' 'exec'),
    (Link 'switch' 'case2' 'probability' 'exec'),
    (Link 'probability' 'hit' 'local_set' 'exec'),
    (Link 'local_set' 'exec' 'return' 'exec'),
    (Link 'local_get' 'value' 'return' 'output_score'),
    (Link 'tier' 'value' 'return' 'output_tier')
)
$scoreVariables = @([ordered]@{ id = 'score_local'; name = 'ScoreLocal'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '函数调用专属局部变量' })
WriteArtifact 'functions/10_score_kernel.obpf' (NativeDocument '评分核心' $scoreNodes $scoreLinks @(
    (New-GraphGroup 'score-main' '函数主路径：输入、计算、分支和返回' 35 55 1740 240 @('entry','sequence','add','multiply','range','switch','probability','tier','return')),
    (New-GraphGroup 'score-local' '局部变量隔离展示' 500 380 600 180 @('local_set','local_get'))
) $scoreVariables ([ordered]@{ functionId = 'functions/10_score_kernel.obpf'; functionCategory = '验证函数'; functionSignature = $scoreSignature }))

# 11: 数组折叠函数，含嵌套循环、数组读取、字符串格式化和对评分函数的嵌套调用陈列。
$arrayFunctionNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/11_array_fold_and_format.obpf' '数组折叠与格式化' $arraySignature)),
    (Node 'sequence' 'origin.flow.sequence' 210 130 @{} @{ label = '折叠函数序列'; dynamicOutputCount = 1 }),
    (Node 'init_sum' 'origin.variable.set' 340 120 @{ value = 0 } @{ variableId = 'fold_sum'; variableAccess = 'set'; label = 'Initialize FoldSum' }),
    (Node 'loop' 'origin.flow.foreach-integer-array' 560 120),
    (Node 'fold_get' 'origin.variable.get' 790 50 @{} @{ variableId = 'fold_sum'; variableAccess = 'get'; label = 'Get FoldSum' }),
    (Node 'weighted_value' 'origin.math.multiply-integer' 790 150),
    (Node 'sum' 'origin.math.add-integer' 1010 120),
    (Node 'fold_set' 'origin.variable.set' 1230 120 @{} @{ variableId = 'fold_sum'; variableAccess = 'set'; label = 'Set FoldSum' }),
    (Node 'get_item' 'origin.array.get-integer' 790 330 @{ index = 1 }),
    (Node 'append' 'origin.array.append-integer' 1010 330 @{ value = 8 }),
    (Node 'array_length' 'origin.array.length' 1230 330),
    (Node 'cast' 'origin.cast.integer-string' 1230 500),
    (Node 'summary' 'origin.literal.string' 1230 610 @{ value = 'weighted-array-fold' }),
    (Node 'score_call' 'origin.function.call' 790 500 @{ input_base = 5; input_bonus = 3; input_multiplier = 2 } (FunctionCallProperties 'functions/10_score_kernel.obpf' '评分核心' $scoreSignature)),
    (Node 'debug' 'origin.debug.output' 1450 430 @{}),
    (Node 'return' 'origin.function.return' 1670 130 @{} (FunctionProperties 'Return' 'functions/11_array_fold_and_format.obpf' '数组折叠与格式化' $arraySignature))
)
$arrayFunctionLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'init_sum' 'exec'),
    (Link 'init_sum' 'exec' 'loop' 'exec'),
    (Link 'entry' 'input_items' 'loop' 'array'),
    (Link 'entry' 'input_items' 'get_item' 'array'),
    (Link 'entry' 'input_items' 'append' 'array'),
    (Link 'loop' 'body' 'fold_set' 'exec'),
    (Link 'fold_get' 'value' 'sum' 'a'),
    (Link 'loop' 'value' 'weighted_value' 'a'),
    (Link 'entry' 'input_weight' 'weighted_value' 'b'),
    (Link 'weighted_value' 'result' 'sum' 'b'),
    (Link 'sum' 'result' 'fold_set' 'value'),
    (Link 'loop' 'completed' 'score_call' 'exec'),
    (Link 'score_call' 'exec' 'debug' 'exec'),
    (Link 'debug' 'exec' 'return' 'exec'),
    (Link 'fold_get' 'value' 'return' 'output_sum'),
    (Link 'get_item' 'value' 'append' 'value'),
    (Link 'append' 'array' 'array_length' 'array'),
    (Link 'array_length' 'length' 'debug' 'integer'),
    (Link 'append' 'array' 'debug' 'array'),
    (Link 'summary' 'value' 'debug' 'string'),
    (Link 'score_call' 'output_score' 'cast' 'value'),
    (Link 'cast' 'result' 'return' 'output_summary')
)
$arrayFunctionVariables = @([ordered]@{ id = 'fold_sum'; name = 'FoldSum'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '数组折叠函数局部累计值' })
WriteArtifact 'functions/11_array_fold_and_format.obpf' (NativeDocument '数组折叠与格式化' $arrayFunctionNodes $arrayFunctionLinks @(
    (New-GraphGroup 'fold-main' '加权数组折叠：初始化、循环累计并返回总和' 35 20 1840 400 @('entry','sequence','init_sum','loop','fold_get','weighted_value','sum','fold_set','get_item','append','array_length','return')),
    (New-GraphGroup 'fold-nested' '嵌套函数调用、格式化和调试输出' 720 450 1020 260 @('score_call','cast','summary','debug'))
) $arrayFunctionVariables ([ordered]@{ functionId = 'functions/11_array_fold_and_format.obpf'; functionCategory = '验证函数'; functionSignature = $arraySignature }))

# 12: 嵌套控制流函数，主执行返回固定 count/trace，控制流组展示循环、break、Range 和 Switch。
$controlFunctionNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/12_nested_control_function.obpf' '嵌套控制流' $controlSignature)),
    (Node 'sequence' 'origin.flow.sequence' 330 120 @{} @{ label = '函数序列'; dynamicOutputCount = 4 }),
    (Node 'outer' 'origin.flow.for-loop' 570 80 @{ start = 0; end = 3 }),
    (Node 'inner_values' 'origin.array.create-integer-new' 570 250 @{ items = @(1, 2, 3) }),
    (Node 'inner' 'origin.flow.foreach-integer-array' 820 80),
    (Node 'break' 'origin.flow.for-loop-break' 570 470 @{ start = 0; end = 4 }),
    (Node 'break_compare' 'origin.compare.greater-integer' 820 440 @{ b = 1 }),
    (Node 'break_branch' 'origin.flow.branch' 1070 440),
    (Node 'break_debug' 'origin.debug.output' 1290 440 @{ string = 'break-loop-body' }),
    (Node 'break_done' 'origin.debug.output' 1290 520 @{ string = 'break-loop-complete' }),
    (Node 'while' 'origin.flow.while' 820 600 @{ condition = $false }),
    (Node 'while_done' 'origin.debug.output' 1070 600 @{ string = 'while-complete' }),
    (Node 'range' 'origin.flow.range-compare' 1070 80 @{ value = 4; ranges = @(2, 4, 8) }),
    (Node 'switch' 'origin.flow.equal-switch-new' 1070 300 @{ value = 2; cases = @(1, 2, 3) }),
    (Node 'trace' 'origin.literal.string' 1320 300 @{ value = 'nested-control:complete' }),
    (Node 'count' 'origin.math.add-integer' 1320 80 @{ a = 3; b = 6 }),
    (Node 'return' 'origin.function.return' 1570 130 @{} (FunctionProperties 'Return' 'functions/12_nested_control_function.obpf' '嵌套控制流' $controlSignature))
)
$controlFunctionLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'count' 'result' 'return' 'output_count'),
    (Link 'trace' 'value' 'return' 'output_trace'),
    (Link 'sequence' 'then0' 'outer' 'exec'),
    (Link 'outer' 'body' 'inner' 'exec'),
    (Link 'inner_values' 'array' 'inner' 'array'),
    (Link 'sequence' 'then1' 'break' 'exec'),
    (Link 'break' 'body' 'break_branch' 'exec'),
    (Link 'break' 'index' 'break_compare' 'a'),
    (Link 'break_compare' 'result' 'break_branch' 'condition'),
    (Link 'break_branch' 'true' 'break' 'break'),
    (Link 'break_branch' 'false' 'break_debug' 'exec'),
    (Link 'break' 'completed' 'break_done' 'exec'),
    (Link 'sequence' 'then2' 'while' 'exec'),
    (Link 'while' 'completed' 'while_done' 'exec'),
    (Link 'sequence' 'then3' 'range' 'exec'),
    (Link 'range' 'case2' 'switch' 'exec'),
    (Link 'switch' 'case2' 'return' 'exec')
)
WriteArtifact 'functions/12_nested_control_function.obpf' (NativeDocument '嵌套控制流' $controlFunctionNodes $controlFunctionLinks @(
    (New-GraphGroup 'nested-flow' '嵌套 Sequence、循环、真实 break 和 while' 35 35 1500 720 @('entry','sequence','outer','inner_values','inner','break','break_compare','break_branch','break_debug','break_done','while','while_done','range','switch')),
    (New-GraphGroup 'function-result' '确定性函数返回值' 1260 35 560 420 @('trace','count','return'))
) @() ([ordered]@{ functionId = 'functions/12_nested_control_function.obpf'; functionCategory = '验证函数'; functionSignature = $controlSignature }))

# 13: 局部状态隔离函数。相同输入多次调用时应只看到本次函数实例的变量初始值。
$localFunctionNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/13_local_state_isolation.obpf' '局部状态隔离' $localSignature)),
    (Node 'sequence' 'origin.flow.sequence' 210 130 @{} @{ label = '局部状态序列'; dynamicOutputCount = 2 }),
    (Node 'get_before' 'origin.variable.get' 340 120 @{} @{ variableId = 'call_counter'; variableAccess = 'get'; label = 'Get CallCounter' }),
    (Node 'add' 'origin.math.add-integer' 580 120),
    (Node 'set_after' 'origin.variable.set' 800 120 @{} @{ variableId = 'call_counter'; variableAccess = 'set'; label = 'Set CallCounter' }),
    (Node 'array' 'origin.array.create-integer-new' 580 330 @{ items = @(1, 1, 2, 3, 5) }),
    (Node 'loop' 'origin.flow.foreach-integer-array' 800 330),
    (Node 'loop_compare' 'origin.compare.greater-integer' 1040 300 @{ b = 2 }),
    (Node 'branch' 'origin.flow.branch' 1260 330),
    (Node 'loop_true_debug' 'origin.debug.output' 1480 280 @{ string = 'local-loop-index-at-least-2' }),
    (Node 'loop_false_debug' 'origin.debug.output' 1480 380 @{ string = 'local-loop-index-below-2' }),
    (Node 'return' 'origin.function.return' 1270 130 @{} (FunctionProperties 'Return' 'functions/13_local_state_isolation.obpf' '局部状态隔离' $localSignature))
)
$localFunctionLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'loop' 'exec'),
    (Link 'sequence' 'then1' 'set_after' 'exec'),
    (Link 'entry' 'input_seed' 'add' 'a'),
    (Link 'get_before' 'value' 'add' 'b'),
    (Link 'add' 'result' 'set_after' 'value'),
    (Link 'set_after' 'exec' 'return' 'exec'),
    (Link 'set_after' 'value' 'return' 'output_result'),
    (Link 'array' 'array' 'loop' 'array'),
    (Link 'loop' 'body' 'branch' 'exec'),
    (Link 'loop' 'index' 'loop_compare' 'a'),
    (Link 'loop_compare' 'result' 'branch' 'condition'),
    (Link 'branch' 'true' 'loop_true_debug' 'exec'),
    (Link 'branch' 'false' 'loop_false_debug' 'exec')
)
$localFunctionVariables = @([ordered]@{ id = 'call_counter'; name = 'CallCounter'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '每次函数调用都必须重新初始化的局部变量' })
WriteArtifact 'functions/13_local_state_isolation.obpf' (NativeDocument '局部状态隔离' $localFunctionNodes $localFunctionLinks @(
    (New-GraphGroup 'local-main' '局部变量读写与函数返回' 35 50 1500 240 @('entry','sequence','get_before','add','set_after','return')),
    (New-GraphGroup 'local-branch' '函数内部数组循环、比较与双分支' 520 250 1240 300 @('array','loop','loop_compare','branch','loop_true_debug','loop_false_debug'))
) $localFunctionVariables ([ordered]@{ functionId = 'functions/13_local_state_isolation.obpf'; functionCategory = '验证函数'; functionSignature = $localSignature }))

# 14: 函数内部挂起与恢复。用于确认函数帧、输入和返回端口在异步恢复后保持不变。
$asyncDelayFunctionNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/14_async_delay_function.obpf' '函数内异步 Delay' $asyncDelaySignature)),
    (Node 'delay' 'origin.example.mock-delay-async' 430 110 @{} $mockDelayFallbackProperties),
    (Node 'return' 'origin.function.return' 820 130 @{} (FunctionProperties 'Return' 'functions/14_async_delay_function.obpf' '函数内异步 Delay' $asyncDelaySignature))
)
$asyncDelayFunctionLinks = @(
    (Link 'entry' 'exec' 'delay' 'exec'),
    (Link 'entry' 'input_delayMs' 'delay' 'delayMs'),
    (Link 'entry' 'input_value' 'delay' 'value'),
    (Link 'entry' 'input_tag' 'delay' 'tag'),
    (Link 'delay' 'completed' 'return' 'exec'),
    (Link 'delay' 'value' 'return' 'output_value'),
    (Link 'delay' 'tag' 'return' 'output_tag')
)
WriteArtifact 'functions/14_async_delay_function.obpf' (NativeDocument '函数内异步 Delay' $asyncDelayFunctionNodes $asyncDelayFunctionLinks @(
    (New-GraphGroup 'async-function' '函数入口 -> 挂起 -> 原函数帧恢复 -> 返回' 35 55 1080 260 @('entry','delay','return'))
) @() ([ordered]@{ functionId = 'functions/14_async_delay_function.obpf'; functionCategory = '验证函数'; functionSignature = $asyncDelaySignature }))

# 05: 主图编排所有函数，并显式连续两次调用局部状态函数。
$orchestratorNodes = @(
    (Node 'entry' 'origin.event.entry-two-integers' 80 130),
    (Node 'sequence' 'origin.flow.sequence' 300 130 @{} @{ label = '函数编排序列'; dynamicOutputCount = 4 }),
    (Node 'score_call' 'origin.function.call' 540 80 @{ input_base = 10; input_bonus = 5; input_multiplier = 2 } (FunctionCallProperties 'functions/10_score_kernel.obpf' '评分核心' $scoreSignature)),
    (Node 'score_return' 'origin.result.append-integer' 830 80),
    (Node 'score_text' 'origin.result.append-string' 1040 80),
    (Node 'array_source' 'origin.array.create-integer-new' 540 330 @{ items = @(3, 1, 4, 1, 5) }),
    (Node 'array_call' 'origin.function.call' 800 330 @{ input_weight = 2 } (FunctionCallProperties 'functions/11_array_fold_and_format.obpf' '数组折叠与格式化' $arraySignature)),
    (Node 'array_return' 'origin.result.append-integer' 1090 330),
    (Node 'summary_return' 'origin.result.append-string' 1300 330),
    (Node 'control_call' 'origin.function.call' 540 580 @{ input_start = 0; input_limit = 4 } (FunctionCallProperties 'functions/12_nested_control_function.obpf' '嵌套控制流' $controlSignature)),
    (Node 'control_count_return' 'origin.result.append-integer' 830 530),
    (Node 'control_return' 'origin.result.append-string' 1050 580),
    (Node 'local_call_a' 'origin.function.call' 540 830 @{ input_seed = 7 } (FunctionCallProperties 'functions/13_local_state_isolation.obpf' '局部状态隔离' $localSignature)),
    (Node 'local_sequence' 'origin.flow.sequence' 810 830 @{} @{ label = '局部调用结果序列'; dynamicOutputCount = 2 }),
    (Node 'local_call_b' 'origin.function.call' 1070 830 @{ input_seed = 7 } (FunctionCallProperties 'functions/13_local_state_isolation.obpf' '局部状态隔离' $localSignature)),
    (Node 'local_return_a' 'origin.result.append-integer' 1350 770),
    (Node 'local_return_b' 'origin.result.append-integer' 1350 900)
)
$orchestratorLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'score_call' 'exec'),
    (Link 'score_call' 'exec' 'score_return' 'exec'),
    (Link 'score_call' 'output_score' 'score_return' 'value'),
    (Link 'score_return' 'exec' 'score_text' 'exec'),
    (Link 'score_call' 'output_tier' 'score_text' 'value'),
    (Link 'sequence' 'then1' 'array_call' 'exec'),
    (Link 'array_source' 'array' 'array_call' 'input_items'),
    (Link 'array_call' 'exec' 'array_return' 'exec'),
    (Link 'array_call' 'output_sum' 'array_return' 'value'),
    (Link 'array_return' 'exec' 'summary_return' 'exec'),
    (Link 'array_call' 'output_summary' 'summary_return' 'value'),
    (Link 'sequence' 'then2' 'control_call' 'exec'),
    (Link 'control_call' 'exec' 'control_count_return' 'exec'),
    (Link 'control_call' 'output_count' 'control_count_return' 'value'),
    (Link 'control_count_return' 'exec' 'control_return' 'exec'),
    (Link 'control_call' 'output_trace' 'control_return' 'value'),
    (Link 'sequence' 'then3' 'local_call_a' 'exec'),
    (Link 'local_call_a' 'exec' 'local_sequence' 'exec'),
    (Link 'local_sequence' 'then0' 'local_return_a' 'exec'),
    (Link 'local_sequence' 'then1' 'local_call_b' 'exec'),
    (Link 'local_call_a' 'output_result' 'local_return_a' 'value'),
    (Link 'local_call_b' 'exec' 'local_return_b' 'exec'),
    (Link 'local_call_b' 'output_result' 'local_return_b' 'value')
)
WriteArtifact '05_function_orchestrator.obp' (NativeDocument '函数编排主图' $orchestratorNodes $orchestratorLinks @(
    (New-GraphGroup 'score-call' '评分函数调用' 35 30 1160 220 @('entry','sequence','score_call','score_return','score_text')),
    (New-GraphGroup 'array-call' '数组折叠函数调用' 480 280 1050 230 @('array_source','array_call','array_return','summary_return')),
    (New-GraphGroup 'control-call' '嵌套控制流函数调用：返回计数和轨迹' 480 500 850 230 @('control_call','control_count_return','control_return')),
    (New-GraphGroup 'local-isolation' '同输入连续调用：局部状态必须隔离' 480 740 1100 250 @('local_call_a','local_sequence','local_call_b','local_return_a','local_return_b'))
))

# 06: 测试专用异步 Delay。单入口覆盖所有循环上下文；截止时间排序与取消由函数图的多个独立 Execution 验证。
$asyncDelayVariables = @(
    [ordered]@{ id = 'while_counter'; name = 'WhileCounter'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = 'While 每次异步恢复后递增，验证恢复点不会重复执行当前迭代' }
)
$asyncDelayNodes = @(
    (Node 'loop_entry' 'origin.event.entry-two-integers' 80 120),
    (Node 'loop_sequence' 'origin.flow.sequence' 320 120 @{} @{ label = '循环异步恢复序列'; dynamicOutputCount = 5 }),

    (Node 'nested_outer' 'origin.flow.for-loop' 600 70 @{ start = 0; end = 2 }),
    (Node 'nested_values' 'origin.array.create-integer-new' 600 250 @{ items = @(2, 4, 6) }),
    (Node 'nested_inner' 'origin.flow.foreach-integer-array' 880 70),
    (Node 'nested_delay' 'origin.example.mock-delay-async' 1160 70 @{ delayMs = 10; value = 0; tag = 'nested-loop' } $mockDelayFallbackProperties),
    (Node 'nested_value_return' 'origin.result.append-integer' 1520 45),
    (Node 'nested_tag_return' 'origin.result.append-string' 1750 100),
    (Node 'nested_done' 'origin.result.append-string' 1520 250 @{ value = 'nested-loop:completed' }),

    (Node 'any_values' 'origin.array.create-string-new' 600 470 @{ items = @('alpha', 'beta', 'gamma') }),
    (Node 'any_loop' 'origin.flow.foreach-array' 880 430),
    (Node 'any_delay' 'origin.example.mock-delay-async' 1160 430 @{ delayMs = 10; value = 0; tag = 'foreach-any' } $mockDelayFallbackProperties),
    (Node 'any_value_return' 'origin.result.append-integer' 1520 405),
    (Node 'any_tag_return' 'origin.result.append-string' 1750 460),
    (Node 'any_done' 'origin.result.append-string' 1520 590 @{ value = 'foreach-any:completed' }),

    (Node 'break_loop' 'origin.flow.for-loop-break' 600 780 @{ start = 0; end = 5 }),
    (Node 'break_delay' 'origin.example.mock-delay-async' 880 750 @{ delayMs = 10; value = 0; tag = 'break-loop' } $mockDelayFallbackProperties),
    (Node 'break_compare' 'origin.compare.greater-integer' 1240 700 @{ b = 1 }),
    (Node 'break_branch' 'origin.flow.branch' 1480 750),
    (Node 'break_value_return' 'origin.result.append-integer' 1740 820),
    (Node 'break_tag_return' 'origin.result.append-string' 1970 850),
    (Node 'break_done' 'origin.result.append-string' 1740 680 @{ value = 'break-loop:completed' }),

    (Node 'while_init' 'origin.variable.set' 600 1110 @{ value = 0 } @{ variableId = 'while_counter'; variableAccess = 'set'; label = 'Initialize WhileCounter' }),
    (Node 'while_loop' 'origin.flow.while' 880 1080),
    (Node 'while_get' 'origin.variable.get' 880 1260 @{} @{ variableId = 'while_counter'; variableAccess = 'get'; label = 'Get WhileCounter' }),
    (Node 'while_condition' 'origin.compare.greater-integer' 1160 1260),
    (Node 'while_delay' 'origin.example.mock-delay-async' 1160 1050 @{ delayMs = 10; value = 0; tag = 'while-loop' } $mockDelayFallbackProperties),
    (Node 'while_value_return' 'origin.result.append-integer' 1520 1020),
    (Node 'while_tag_return' 'origin.result.append-string' 1750 1070),
    (Node 'while_add' 'origin.math.add-integer' 1980 1200 @{ b = 1 }),
    (Node 'while_set' 'origin.variable.set' 2220 1090 @{} @{ variableId = 'while_counter'; variableAccess = 'set'; label = 'Increment WhileCounter' }),
    (Node 'while_done' 'origin.result.append-string' 1520 1350 @{ value = 'while-loop:completed' }),

    (Node 'function_call' 'origin.function.call' 600 1530 @{ input_delayMs = 10; input_value = 900; input_tag = 'function-delay' } (FunctionCallProperties 'functions/14_async_delay_function.obpf' '函数内异步 Delay' $asyncDelaySignature)),
    (Node 'function_value_return' 'origin.result.append-integer' 980 1500),
    (Node 'function_tag_return' 'origin.result.append-string' 1210 1550)
)
$asyncDelayLinks = @(
    (Link 'loop_entry' 'exec' 'loop_sequence' 'exec'),

    (Link 'loop_sequence' 'then0' 'nested_outer' 'exec'),
    (Link 'loop_entry' 'param1' 'nested_outer' 'end'),
    (Link 'nested_outer' 'body' 'nested_inner' 'exec'),
    (Link 'nested_values' 'array' 'nested_inner' 'array'),
    (Link 'nested_inner' 'body' 'nested_delay' 'exec'),
    (Link 'loop_entry' 'param2' 'nested_delay' 'delayMs'),
    (Link 'nested_inner' 'value' 'nested_delay' 'value'),
    (Link 'nested_delay' 'completed' 'nested_value_return' 'exec'),
    (Link 'nested_delay' 'value' 'nested_value_return' 'value'),
    (Link 'nested_value_return' 'exec' 'nested_tag_return' 'exec'),
    (Link 'nested_delay' 'tag' 'nested_tag_return' 'value'),
    (Link 'nested_outer' 'completed' 'nested_done' 'exec'),

    (Link 'loop_sequence' 'then1' 'any_loop' 'exec'),
    (Link 'any_values' 'array' 'any_loop' 'array'),
    (Link 'any_loop' 'body' 'any_delay' 'exec'),
    (Link 'loop_entry' 'param2' 'any_delay' 'delayMs'),
    (Link 'any_loop' 'index' 'any_delay' 'value'),
    (Link 'any_delay' 'completed' 'any_value_return' 'exec'),
    (Link 'any_delay' 'value' 'any_value_return' 'value'),
    (Link 'any_value_return' 'exec' 'any_tag_return' 'exec'),
    (Link 'any_delay' 'tag' 'any_tag_return' 'value'),
    (Link 'any_loop' 'completed' 'any_done' 'exec'),

    (Link 'loop_sequence' 'then2' 'break_loop' 'exec'),
    (Link 'loop_entry' 'param1' 'break_loop' 'end'),
    (Link 'break_loop' 'body' 'break_delay' 'exec'),
    (Link 'loop_entry' 'param2' 'break_delay' 'delayMs'),
    (Link 'break_loop' 'index' 'break_delay' 'value'),
    (Link 'break_delay' 'completed' 'break_branch' 'exec'),
    (Link 'break_delay' 'value' 'break_compare' 'a'),
    (Link 'break_compare' 'result' 'break_branch' 'condition'),
    (Link 'break_branch' 'true' 'break_loop' 'break'),
    (Link 'break_branch' 'false' 'break_value_return' 'exec'),
    (Link 'break_delay' 'value' 'break_value_return' 'value'),
    (Link 'break_value_return' 'exec' 'break_tag_return' 'exec'),
    (Link 'break_delay' 'tag' 'break_tag_return' 'value'),
    (Link 'break_loop' 'completed' 'break_done' 'exec'),

    (Link 'loop_sequence' 'then3' 'while_init' 'exec'),
    (Link 'while_init' 'exec' 'while_loop' 'exec'),
    (Link 'loop_entry' 'param1' 'while_condition' 'a'),
    (Link 'while_get' 'value' 'while_condition' 'b'),
    (Link 'while_condition' 'result' 'while_loop' 'condition'),
    (Link 'while_loop' 'body' 'while_delay' 'exec'),
    (Link 'loop_entry' 'param2' 'while_delay' 'delayMs'),
    (Link 'while_get' 'value' 'while_delay' 'value'),
    (Link 'while_delay' 'completed' 'while_value_return' 'exec'),
    (Link 'while_delay' 'value' 'while_value_return' 'value'),
    (Link 'while_value_return' 'exec' 'while_tag_return' 'exec'),
    (Link 'while_delay' 'tag' 'while_tag_return' 'value'),
    (Link 'while_tag_return' 'exec' 'while_set' 'exec'),
    (Link 'while_get' 'value' 'while_add' 'a'),
    (Link 'while_add' 'result' 'while_set' 'value'),
    (Link 'while_loop' 'completed' 'while_done' 'exec'),

    (Link 'loop_sequence' 'then4' 'function_call' 'exec'),
    (Link 'loop_entry' 'param2' 'function_call' 'input_delayMs'),
    (Link 'loop_entry' 'objectId' 'function_call' 'input_value'),
    (Link 'function_call' 'exec' 'function_value_return' 'exec'),
    (Link 'function_call' 'output_value' 'function_value_return' 'value'),
    (Link 'function_value_return' 'exec' 'function_tag_return' 'exec'),
    (Link 'function_call' 'output_tag' 'function_tag_return' 'value')
)
WriteArtifact '06_async_delay_resume.obp' (NativeDocument '异步 Delay 恢复验证' $asyncDelayNodes $asyncDelayLinks @(
    (New-GraphGroup 'nested-loop-delay' '嵌套 For + ForeachIntArray：每次恢复后只进入下一迭代' 35 35 2050 310 @('loop_entry','loop_sequence','nested_outer','nested_values','nested_inner','nested_delay','nested_value_return','nested_tag_return','nested_done')),
    (New-GraphGroup 'foreach-any-delay' 'ForeachArray：任意数组循环内挂起与恢复' 540 380 1545 300 @('any_values','any_loop','any_delay','any_value_return','any_tag_return','any_done')),
    (New-GraphGroup 'break-loop-delay' 'ForLoopWithBreak：恢复后判断 break，不重复或多跑迭代' 540 650 1700 360 @('break_loop','break_delay','break_compare','break_branch','break_value_return','break_tag_return','break_done')),
    (New-GraphGroup 'while-delay' 'While：恢复后递增计数，再重新判断下一轮条件' 540 1000 1960 500 @('while_init','while_loop','while_get','while_condition','while_delay','while_value_return','while_tag_return','while_add','while_set','while_done')),
    (New-GraphGroup 'function-delay' '函数调用内部挂起：恢复后回到原函数帧并返回输出' 540 1460 1000 260 @('function_call','function_value_return','function_tag_return'))
) $asyncDelayVariables)

# 07: 使用测试节点模拟 RPC 回包。单入口依次验证成功和失败 ResumeTo 出口。
$mockRpcNodes = @(
    (Node 'success_entry' 'origin.event.entry-two-integers' 80 150),
    (Node 'success_rpc' 'origin.example.mock-rpc-async' 380 120 @{ delayMs = 80; succeed = $true; successValue = 314; failureCode = 0; failureMessage = '' } $mockRpcFallbackProperties),
    (Node 'success_return' 'origin.result.append-integer' 790 90),
    (Node 'success_unexpected_text' 'origin.literal.string' 710 280 @{ value = 'unexpected failure from success request' }),
    (Node 'success_unexpected_return' 'origin.result.append-string' 990 230),
    (Node 'failure_rpc' 'origin.example.mock-rpc-async' 1180 90 @{ delayMs = 80; succeed = $false; successValue = 0; failureCode = 503; failureMessage = 'mock rpc unavailable' } $mockRpcFallbackProperties),
    (Node 'failure_unexpected_return' 'origin.result.append-integer' 1570 40),
    (Node 'failure_code_return' 'origin.result.append-integer' 1570 190),
    (Node 'failure_return' 'origin.result.append-string' 1810 190)
)
$mockRpcLinks = @(
    (Link 'success_entry' 'exec' 'success_rpc' 'exec'),
    (Link 'success_rpc' 'succeeded' 'success_return' 'exec'),
    (Link 'success_rpc' 'value' 'success_return' 'value'),
    (Link 'success_rpc' 'failed' 'success_unexpected_return' 'exec'),
    (Link 'success_unexpected_text' 'value' 'success_unexpected_return' 'value'),
    (Link 'success_return' 'exec' 'failure_rpc' 'exec'),
    (Link 'failure_rpc' 'succeeded' 'failure_unexpected_return' 'exec'),
    (Link 'failure_rpc' 'value' 'failure_unexpected_return' 'value'),
    (Link 'failure_rpc' 'failed' 'failure_code_return' 'exec'),
    (Link 'failure_rpc' 'errorCode' 'failure_code_return' 'value'),
    (Link 'failure_code_return' 'exec' 'failure_return' 'exec'),
    (Link 'failure_rpc' 'errorMessage' 'failure_return' 'value')
)
WriteArtifact '07_async_rpc_resume_to.obp' (NativeDocument '定时器模拟 RPC 异步恢复' $mockRpcNodes $mockRpcLinks @(
    (New-GraphGroup 'success-rpc' '单入口第一步：ResumeTo(成功) 返回成功值' 35 35 1080 350 @('success_entry','success_rpc','success_return','success_unexpected_text','success_unexpected_return')),
    (New-GraphGroup 'failure-rpc' '成功步骤完成后：ResumeTo(失败) 返回错误码和错误文本' 1120 35 900 350 @('failure_rpc','failure_unexpected_return','failure_code_return','failure_return'))
))

$coverage = [ordered]@{
    schemaVersion = 1
    description = '顶层系统节点的样本覆盖矩阵。visual 表示第 1 阶段人工检查；execution/async 留给后续阶段。'
    nodes = [ordered]@{
        'origin.event.entry-array' = @('03_array_data_lab.obp:visual')
        'origin.event.entry-two-integers' = @('02_control_flow_maze.obp:execution','04_deterministic_algorithm.obp:execution','05_function_orchestrator.obp:execution','06_async_delay_resume.obp:async')
        'origin.debug.output' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:visual')
        'origin.cast.integer-string' = @('04_deterministic_algorithm.obp:visual')
        'origin.cast.float-string' = @('04_deterministic_algorithm.obp:visual')
        'origin.cast.any-string' = @('02_control_flow_maze.obp:visual','03_array_data_lab.obp:visual')
        'origin.literal.string' = @('03_array_data_lab.obp:visual','functions/10_score_kernel.obpf:visual')
        'origin.math.add-integer' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','04_deterministic_algorithm.obp:execution')
        'origin.math.subtract-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:execution')
        'origin.math.multiply-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:execution')
        'origin.math.divide-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:execution')
        'origin.math.modulo-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:execution')
        'origin.math.random-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:execution')
        'origin.math.add-float' = @('04_deterministic_algorithm.obp:visual')
        'origin.math.subtract-float' = @('04_deterministic_algorithm.obp:visual')
        'origin.math.multiply-float' = @('04_deterministic_algorithm.obp:visual')
        'origin.math.divide-float' = @('04_deterministic_algorithm.obp:visual')
        'origin.compare.greater-integer' = @('04_deterministic_algorithm.obp:execution')
        'origin.flow.sequence' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','06_async_delay_resume.obp:async','functions/12_nested_control_function.obpf:visual')
        'origin.flow.for-loop' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','06_async_delay_resume.obp:async')
        'origin.flow.branch' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution')
        'origin.flow.greater-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.less-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.equal-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.foreach-integer-array' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','06_async_delay_resume.obp:async')
        'origin.flow.while' = @('02_control_flow_maze.obp:execution','06_async_delay_resume.obp:async','functions/12_nested_control_function.obpf:visual')
        'origin.flow.for-loop-break' = @('02_control_flow_maze.obp:execution','06_async_delay_resume.obp:async','functions/12_nested_control_function.obpf:visual')
        'origin.flow.foreach-array' = @('02_control_flow_maze.obp:visual','06_async_delay_resume.obp:async')
        'origin.flow.probability' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:execution','functions/10_score_kernel.obpf:visual')
        'origin.flow.range-compare' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution')
        'origin.flow.equal-switch' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:visual')
        'origin.flow.equal-switch-new' = @('02_control_flow_maze.obp:visual','04_deterministic_algorithm.obp:execution')
        'origin.array.get-integer' = @('03_array_data_lab.obp:visual','functions/11_array_fold_and_format.obpf:visual')
        'origin.array.get-string' = @('03_array_data_lab.obp:visual')
        'origin.array.get-any' = @('03_array_data_lab.obp:visual')
        'origin.array.length' = @('03_array_data_lab.obp:execution')
        'origin.array.create-integer' = @('03_array_data_lab.obp:visual')
        'origin.array.create-integer-new' = @('02_control_flow_maze.obp:execution','03_array_data_lab.obp:execution')
        'origin.array.create-string' = @('03_array_data_lab.obp:visual')
        'origin.array.create-string-new' = @('02_control_flow_maze.obp:visual','03_array_data_lab.obp:execution')
        'origin.array.append-string' = @('03_array_data_lab.obp:execution')
        'origin.array.append-integer' = @('03_array_data_lab.obp:execution')
        'origin.result.append-integer' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','04_deterministic_algorithm.obp:execution')
        'origin.result.append-string' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','04_deterministic_algorithm.obp:execution')
        'origin.example.mock-delay-async' = @('06_async_delay_resume.obp:async-demo','functions/14_async_delay_function.obpf:async-demo')
        'origin.example.mock-rpc-async' = @('07_async_rpc_resume_to.obp:async-demo')
        'origin.string.split' = @('03_array_data_lab.obp:visual')
        'origin.variable.get' = @('03_array_data_lab.obp:visual','06_async_delay_resume.obp:async','functions/13_local_state_isolation.obpf:visual')
        'origin.variable.set' = @('03_array_data_lab.obp:visual','06_async_delay_resume.obp:async','functions/13_local_state_isolation.obpf:visual')
        'origin.function.entry' = @('functions/10_score_kernel.obpf:visual','functions/11_array_fold_and_format.obpf:visual','functions/12_nested_control_function.obpf:visual','functions/13_local_state_isolation.obpf:visual','functions/14_async_delay_function.obpf:async')
        'origin.function.return' = @('functions/10_score_kernel.obpf:execution','functions/11_array_fold_and_format.obpf:execution','functions/12_nested_control_function.obpf:execution','functions/13_local_state_isolation.obpf:execution','functions/14_async_delay_function.obpf:async')
        'origin.function.call' = @('05_function_orchestrator.obp:execution','06_async_delay_resume.obp:async','functions/11_array_fold_and_format.obpf:visual')
    }
}
WriteArtifact 'coverage.json' $coverage

$readme = @'
# 蓝图验证样本

这些文件既用于 OriginBlueprint 的人工可视化检查，也由 Go 自动化测试加载执行。测试会将每个蓝图的实际返回值与独立 Go 参考实现比较。

## 建议打开顺序

1. `01_legacy_all_nodes_showcase.vgf`：确认 legacy 节点、默认值和旧端口在新编辑器中的迁移显示。
2. `02_control_flow_maze.obp`：确认 Sequence、嵌套循环、动态分支、while、break 与任意数组循环的布局和连线。
3. `03_array_data_lab.obp`：确认数组控件、字符串控件、转换节点和局部变量节点。
4. `04_deterministic_algorithm.obp`：确认算术、浮点、比较、Branch、Range、Switch 和固定随机数的端口。
5. 打开 `functions/` 下五个 `.obpf`：确认函数入口/返回的参数名、类型、函数内变量和异步恢复端口显示。
6. `05_function_orchestrator.obp`：确认外部函数调用节点的输入输出端口，以及连续两次调用局部状态函数的可读性。
7. `06_async_delay_resume.obp`：确认所有循环内挂起恢复和函数内挂起的显示与完整连线；再打开 `functions/14_async_delay_function.obpf` 确认可独立传入延迟、整数和标记。
8. `07_async_rpc_resume_to.obp`：确认单一入口依次执行成功、失败两次异步回包，并展示两个 ResumeTo 出口的连线。

## 关键预期

- 函数入口、函数返回和函数调用节点的参数端口名称必须完整显示。
- 所有动态 Sequence、Range 与 Switch 的已连接分支端口必须可见。
- 图中每个分组标题应完整可读，节点不应重叠遮挡端口。
- `13_local_state_isolation.obpf` 的变量属于函数局部状态；`05_function_orchestrator.obp` 连续调用它两次，是后续隔离验证的样本入口。
- `coverage.json` 记录全部当前系统节点的样本位置和阶段覆盖范围。
- `nodes/MockDelayAsync.json` 和 `nodes/MockRpcAsync.json` 是本目录专用测试节点定义，不属于正式系统节点库；其 Go 实现和结果断言位于 `engine/go/blueprint` 的验证测试中。
- `MockDelayAsync` 只表达业务异步节点的 `Yield -> Resume` 语义，不重新引入正式 `Delay`、`Timer` 或 `TimerHandle` 节点。
- `MockDelayAsync` 和 `MockRpcAsync` 节点同时在文档属性中携带测试专用 fallback 端口；这是因为编辑器只扫描根目录 `nodes/`，fallback 仅用于让示例目录中的外部节点和连线可视化，不会将这些节点加入正式模块库。

## 第 2 阶段结果契约

- `01_legacy_all_nodes_showcase.vgf`：只验证 legacy 导入、端口迁移和显示；不作为结果对比图。
- `02_control_flow_maze.obp`：验证嵌套循环、真实 break、Range、Branch、Probability、While 和任意数组遍历。固定输入下会返回循环整数、各分支标记和数组转换字符串。
- `03_array_data_lab.obp`：固定数组应依次返回整数 `4`、长度 `6`、字符串 `green` 和局部变量字符串 `north`。
- `04_deterministic_algorithm.obp`：输入参数决定整数评分分支；固定随机数恒为 `42`，Range/Switch 与浮点转换返回固定文本。后续随机输入阶段会以同一入口参数调用 Go 参考实现。
- `05_function_orchestrator.obp`：验证评分函数、加权数组折叠、嵌套控制函数和两次独立局部状态函数调用的全部输出。
- `06_async_delay_resume.obp`：唯一入口依次验证嵌套 For/ForeachIntArray、ForeachArray、ForLoopWithBreak、While 和函数调用内部的挂起恢复；每次恢复只能继续当前迭代余下语句，随后进入下一迭代。
- `functions/14_async_delay_function.obpf`：Go 测试会对该函数图启动多个独立 Execution，分别传入 10ms、30ms 和 5000ms；验证截止时间顺序，并验证取消 5000ms Execution 后不会恢复。截止时间和取消属于 Execution 调度测试，不应伪装成同一图中的多个同 ID 入口。
- `07_async_rpc_resume_to.obp`：单一入口先从成功分支返回 `314`，再从失败分支返回错误码 `503` 和文本 `mock rpc unavailable`。
- `functions/10_score_kernel.obpf`、`functions/11_array_fold_and_format.obpf`、`functions/12_nested_control_function.obpf`、`functions/13_local_state_isolation.obpf`、`functions/14_async_delay_function.obpf` 分别验证评分、加权累计、真实 break、函数局部变量隔离和函数帧异步恢复。

## 重新生成

在仓库根目录运行：

```powershell
.\scripts\generate-verification-blueprints.cmd
```

请勿在 Windows PowerShell 5 中直接执行 `.ps1`；它可能用系统代码页误读无 BOM 的 UTF-8 中文文本。

## 自动化对比

每个蓝图均已有独立 Go 参考实现。随机对比使用每个文件独立的固定 seed 和 64 组不重复输入，每组重复 3 次；测试会主动拒绝重复输入。异步 Delay 使用虚拟时钟，测试不依赖真实等待。设置 `WRITE_BLUEPRINT_VERIFICATION_REPORT=1` 执行报告测试可更新 `docs/BLUEPRINT_VERIFICATION_MATRIX_ZH.md`。
'@
WriteArtifact 'README.md' $readme

Write-Host "Generated verification blueprints at $OutputRoot"
