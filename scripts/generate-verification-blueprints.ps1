param(
    [string]$OutputRoot = (Join-Path (Split-Path -Parent $PSScriptRoot) 'examples/verification-blueprints')
)

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

# 01: legacy 格式。三个入口分别形成整数、数组控制流和 Timer 事件路径。
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
    (LegacyNode 'entry_timer' 'Entrance_Timer_000003' 80 930),
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
    (LegacyEdge 'entry_timer' 0 'timer_debug' 0),
    (LegacyEdge 'random' 0 'timer_debug' 1)
)
WriteArtifact '01_legacy_all_nodes_showcase.vgf' ([ordered]@{ graph_name = 'Legacy All Nodes Showcase'; time = '2026-07-11T00:00:00Z'; nodes = $legacyNodes; edges = $legacyEdges; groups = @(); variables = @() })

# 02: native 控制流。主路径包含 Sequence、循环、分支、动态分支和 break。
$controlNodes = @(
    (Node 'entry' 'origin.event.entry-two-integers' 80 130),
    (Node 'sequence' 'origin.flow.sequence' 300 130 @{} @{ label = '六路序列'; dynamicOutputCount = 6 }),
    (Node 'outer_loop' 'origin.flow.for-loop' 540 80 @{ start = 0; end = 3 }),
    (Node 'numbers' 'origin.array.create-integer-new' 535 250 @{ items = @(2, 4, 6) }),
    (Node 'inner_loop' 'origin.flow.foreach-integer-array' 780 80),
    (Node 'sum' 'origin.math.add-integer' 1000 80),
    (Node 'loop_return' 'origin.result.append-integer' 1210 80),
    (Node 'range' 'origin.flow.range-compare' 540 430 @{ value = 4; ranges = @(3, 6, 10) }),
    (Node 'branch' 'origin.flow.branch' 780 430 @{ condition = $true }),
    (Node 'range_text' 'origin.result.append-string' 1010 430 @{ value = 'range-branch-true' }),
    (Node 'break_loop' 'origin.flow.for-loop-break' 540 670 @{ start = 0; end = 5 }),
    (Node 'break_value' 'origin.result.append-integer' 780 670),
    (Node 'break_done' 'origin.result.append-string' 1010 670 @{ value = 'break-loop-complete' }),
    (Node 'probability' 'origin.flow.probability' 540 900 @{ probability = 10000 }),
    (Node 'probability_text' 'origin.result.append-string' 780 900 @{ value = 'probability-hit' }),
    (Node 'while' 'origin.flow.while' 1020 900 @{ condition = $false }),
    (Node 'while_done' 'origin.result.append-string' 1240 900 @{ value = 'while-complete' }),
    (Node 'greater' 'origin.flow.greater-integer' 80 1120 @{ orEqual = $false; a = 9; b = 4 }),
    (Node 'less' 'origin.flow.less-integer' 300 1120 @{ orEqual = $true; a = 4; b = 4 }),
    (Node 'equal' 'origin.flow.equal-integer' 520 1120 @{ a = 7; b = 7 }),
    (Node 'switch_legacy' 'origin.flow.equal-switch' 760 1120 @{ value = 7; cases = @(1, 7, 9) }),
    (Node 'switch_new' 'origin.flow.equal-switch-new' 1030 1120 @{ value = 8; cases = @(2, 8, 10) }),
    (Node 'switch_text' 'origin.result.append-string' 1280 1120 @{ value = 'comparison-switch-complete' }),
    (Node 'string_items' 'origin.array.create-string-new' 80 1330 @{ items = @('alpha', 'beta', 'gamma') }),
    (Node 'foreach_any' 'origin.flow.foreach-array' 330 1330),
    (Node 'cast_any' 'origin.cast.any-string' 580 1330),
    (Node 'any_text' 'origin.result.append-string' 840 1330 @{ value = 'foreach-array-visible' })
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
    (Link 'sequence' 'then1' 'range' 'exec'),
    (Link 'range' 'case2' 'branch' 'exec'),
    (Link 'branch' 'true' 'range_text' 'exec'),
    (Link 'sequence' 'then2' 'break_loop' 'exec'),
    (Link 'break_loop' 'body' 'break_value' 'exec'),
    (Link 'break_loop' 'index' 'break_value' 'value'),
    (Link 'break_loop' 'completed' 'break_done' 'exec'),
    (Link 'sequence' 'then3' 'probability' 'exec'),
    (Link 'probability' 'hit' 'probability_text' 'exec'),
    (Link 'probability_text' 'exec' 'while' 'exec'),
    (Link 'while' 'completed' 'while_done' 'exec'),
    (Link 'sequence' 'then4' 'foreach_any' 'exec'),
    (Link 'string_items' 'array' 'foreach_any' 'array'),
    (Link 'foreach_any' 'body' 'cast_any' 'exec'),
    (Link 'foreach_any' 'value' 'cast_any' 'value'),
    (Link 'cast_any' 'exec' 'any_text' 'exec'),
    (Link 'cast_any' 'result' 'any_text' 'value'),
    (Link 'sequence' 'then5' 'greater' 'exec'),
    (Link 'greater' 'true' 'less' 'exec'),
    (Link 'less' 'true' 'equal' 'exec'),
    (Link 'equal' 'true' 'switch_legacy' 'exec'),
    (Link 'switch_legacy' 'case2' 'switch_new' 'exec'),
    (Link 'switch_new' 'case2' 'switch_text' 'exec')
)
WriteArtifact '02_control_flow_maze.obp' (NativeDocument '控制流迷宫' $controlNodes $controlLinks @(
    (New-GraphGroup 'main-flow' '主执行路径：嵌套循环与分支' 35 25 1360 1020 @('entry','sequence','outer_loop','numbers','inner_loop','sum','loop_return','range','branch','range_text','break_loop','break_value','break_done','probability','probability_text','while','while_done')),
    (New-GraphGroup 'flow-gallery' '控制流扩展路径：比较、Switch 与任意数组循环' 35 1070 1530 360 @('greater','less','equal','switch_legacy','switch_new','switch_text','string_items','foreach_any','cast_any','any_text'))
))

# 03: 数组、字符串、变量和转换。主路径以固定数组输入产出可读结果。
$arrayVariables = @(
    [ordered]@{ id = 'array_sum'; name = 'ArraySum'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '数组实验室中的累计值' },
    [ordered]@{ id = 'array_label'; name = 'ArrayLabel'; type = 'string'; defaultValue = 'cold'; groupId = 'local'; description = '局部字符串变量' }
)
$arrayNodes = @(
    (Node 'entry' 'origin.event.entry-array' 80 120),
    (Node 'sequence' 'origin.flow.sequence' 290 120 @{} @{ label = '数组实验序列'; dynamicOutputCount = 5 }),
    (Node 'integers_old' 'origin.array.create-integer' 510 80 @{ items = @(3, 1, 4, 1, 5) }),
    (Node 'integers_new' 'origin.array.create-integer-new' 510 230 @{ items = @(3, 1, 4, 1, 5) }),
    (Node 'append_integer' 'origin.array.append-integer' 760 230 @{ value = 9 }),
    (Node 'get_integer' 'origin.array.get-integer' 1010 230 @{ index = 2 }),
    (Node 'length' 'origin.array.length' 1010 360),
    (Node 'contains' 'origin.variable.set' 1010 500 @{ value = 0 } @{ variableId = 'array_sum'; variableAccess = 'set'; label = 'Set ArraySum' }),
    (Node 'sum_get' 'origin.variable.get' 1220 500 @{} @{ variableId = 'array_sum'; variableAccess = 'get'; label = 'Get ArraySum' }),
    (Node 'integer_return' 'origin.result.append-integer' 1460 230),
    (Node 'length_return' 'origin.result.append-integer' 1460 360),
    (Node 'strings_old' 'origin.array.create-string' 510 700 @{ items = @('red', 'green', 'blue') }),
    (Node 'strings_new' 'origin.array.create-string-new' 510 850 @{ items = @('red', 'green', 'blue') }),
    (Node 'append_string' 'origin.array.append-string' 760 850 @{ value = 'violet' }),
    (Node 'get_string' 'origin.array.get-string' 1010 850 @{ index = 1 }),
    (Node 'string_return' 'origin.result.append-string' 1260 850),
    (Node 'literal' 'origin.literal.string' 510 1130 @{ value = 'east|west|north' }),
    (Node 'split' 'origin.string.split' 760 1130 @{ delimiter = '|' }),
    (Node 'get_any' 'origin.array.get-any' 1010 1130 @{ index = 2 }),
    (Node 'cast_any' 'origin.cast.any-string' 1260 1130),
    (Node 'any_return' 'origin.result.append-string' 1510 1130),
    (Node 'label_set' 'origin.variable.set' 760 1290 @{ value = 'array-lab' } @{ variableId = 'array_label'; variableAccess = 'set'; label = 'Set ArrayLabel' }),
    (Node 'label_get' 'origin.variable.get' 1010 1290 @{} @{ variableId = 'array_label'; variableAccess = 'get'; label = 'Get ArrayLabel' })
)
$arrayLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'integer_return' 'exec'),
    (Link 'integers_new' 'array' 'append_integer' 'array'),
    (Link 'append_integer' 'array' 'integers_old' 'items'),
    (Link 'integers_old' 'array' 'get_integer' 'array'),
    (Link 'get_integer' 'value' 'contains' 'value'),
    (Link 'sum_get' 'value' 'integer_return' 'value'),
    (Link 'sequence' 'then1' 'length_return' 'exec'),
    (Link 'append_integer' 'array' 'length' 'array'),
    (Link 'length' 'length' 'length_return' 'value'),
    (Link 'sequence' 'then2' 'string_return' 'exec'),
    (Link 'strings_new' 'array' 'append_string' 'array'),
    (Link 'append_string' 'array' 'strings_old' 'items'),
    (Link 'strings_old' 'array' 'get_string' 'array'),
    (Link 'get_string' 'value' 'string_return' 'value'),
    (Link 'literal' 'value' 'split' 'text'),
    (Link 'split' 'array' 'get_any' 'array'),
    (Link 'get_any' 'value' 'cast_any' 'value'),
    (Link 'sequence' 'then3' 'cast_any' 'exec'),
    (Link 'cast_any' 'exec' 'label_set' 'exec'),
    (Link 'cast_any' 'result' 'label_set' 'value'),
    (Link 'label_set' 'exec' 'contains' 'exec'),
    (Link 'sequence' 'then4' 'any_return' 'exec'),
    (Link 'label_get' 'value' 'any_return' 'value')
)
WriteArtifact '03_array_data_lab.obp' (NativeDocument '数组数据实验室' $arrayNodes $arrayLinks @(
    (New-GraphGroup 'integer-arrays' '整数数组：创建、追加、读取与长度' 35 30 1600 580 @('entry','sequence','integers_old','integers_new','append_integer','get_integer','length','contains','sum_get','integer_return','length_return')),
    (New-GraphGroup 'string-arrays' '字符串数组：创建、追加和读取' 455 650 980 340 @('strings_old','strings_new','append_string','get_string','string_return')),
    (New-GraphGroup 'text-arrays' '字符串切分、任意项读取和局部变量' 455 1080 1150 290 @('literal','split','get_any','cast_any','any_return','label_set','label_get'))
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
    (Node 'loop' 'origin.flow.foreach-integer-array' 340 120),
    (Node 'sum' 'origin.math.add-integer' 580 120),
    (Node 'count_loop' 'origin.flow.for-loop' 800 120 @{ start = 0; end = 3 }),
    (Node 'get_item' 'origin.array.get-integer' 1040 120 @{ index = 1 }),
    (Node 'append' 'origin.array.append-integer' 1040 340 @{ value = 8 }),
    (Node 'array_length' 'origin.array.length' 1270 340),
    (Node 'cast' 'origin.cast.integer-string' 1250 120),
    (Node 'summary' 'origin.literal.string' 1250 340 @{ value = 'array-fold:fixed' }),
    (Node 'score_call' 'origin.function.call' 800 520 @{ input_base = 5; input_bonus = 3; input_multiplier = 2 } (FunctionCallProperties 'functions/10_score_kernel.obpf' '评分核心' $scoreSignature)),
    (Node 'local_set' 'origin.variable.set' 1060 520 @{ value = 0 } @{ variableId = 'fold_sum'; variableAccess = 'set'; label = 'Set FoldSum' }),
    (Node 'debug' 'origin.debug.output' 1300 520 @{}),
    (Node 'return' 'origin.function.return' 1510 130 @{} (FunctionProperties 'Return' 'functions/11_array_fold_and_format.obpf' '数组折叠与格式化' $arraySignature))
)
$arrayFunctionLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'loop' 'exec'),
    (Link 'entry' 'input_items' 'loop' 'array'),
    (Link 'entry' 'input_items' 'get_item' 'array'),
    (Link 'entry' 'input_items' 'append' 'array'),
    (Link 'loop' 'body' 'count_loop' 'exec'),
    (Link 'count_loop' 'body' 'score_call' 'exec'),
    (Link 'score_call' 'exec' 'debug' 'exec'),
    (Link 'debug' 'exec' 'local_set' 'exec'),
    (Link 'loop' 'completed' 'return' 'exec'),
    (Link 'loop' 'value' 'sum' 'a'),
    (Link 'entry' 'input_weight' 'sum' 'b'),
    (Link 'sum' 'result' 'return' 'output_sum'),
    (Link 'get_item' 'value' 'append' 'value'),
    (Link 'append' 'array' 'array_length' 'array'),
    (Link 'array_length' 'length' 'local_set' 'value'),
    (Link 'array_length' 'length' 'debug' 'integer'),
    (Link 'append' 'array' 'debug' 'array'),
    (Link 'summary' 'value' 'debug' 'string'),
    (Link 'score_call' 'output_score' 'cast' 'value'),
    (Link 'cast' 'result' 'return' 'output_summary')
)
$arrayFunctionVariables = @([ordered]@{ id = 'fold_sum'; name = 'FoldSum'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '数组折叠函数局部累计值' })
WriteArtifact 'functions/11_array_fold_and_format.obpf' (NativeDocument '数组折叠与格式化' $arrayFunctionNodes $arrayFunctionLinks @(
    (New-GraphGroup 'fold-main' '函数主路径与数组循环' 35 50 1740 370 @('entry','sequence','loop','sum','count_loop','get_item','append','array_length','cast','summary','return')),
    (New-GraphGroup 'fold-nested' '嵌套函数调用、调试和局部变量' 720 470 850 180 @('score_call','local_set','debug'))
) $arrayFunctionVariables ([ordered]@{ functionId = 'functions/11_array_fold_and_format.obpf'; functionCategory = '验证函数'; functionSignature = $arraySignature }))

# 12: 嵌套控制流函数，主执行返回固定 count/trace，控制流组展示循环、break、Range 和 Switch。
$controlFunctionNodes = @(
    (Node 'entry' 'origin.function.entry' 80 130 @{} (FunctionProperties 'Entry' 'functions/12_nested_control_function.obpf' '嵌套控制流' $controlSignature)),
    (Node 'sequence' 'origin.flow.sequence' 330 120 @{} @{ label = '函数序列'; dynamicOutputCount = 4 }),
    (Node 'outer' 'origin.flow.for-loop' 570 80 @{ start = 0; end = 3 }),
    (Node 'inner_values' 'origin.array.create-integer-new' 570 250 @{ items = @(1, 2, 3) }),
    (Node 'inner' 'origin.flow.foreach-integer-array' 820 80),
    (Node 'break' 'origin.flow.for-loop-break' 570 470 @{ start = 0; end = 4 }),
    (Node 'while' 'origin.flow.while' 820 470 @{ condition = $false }),
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
    (Link 'sequence' 'then2' 'while' 'exec'),
    (Link 'sequence' 'then3' 'range' 'exec'),
    (Link 'range' 'case2' 'switch' 'exec'),
    (Link 'switch' 'case2' 'return' 'exec')
)
WriteArtifact 'functions/12_nested_control_function.obpf' (NativeDocument '嵌套控制流' $controlFunctionNodes $controlFunctionLinks @(
    (New-GraphGroup 'nested-flow' '嵌套 Sequence、循环、break 和 while' 35 35 1160 650 @('entry','sequence','outer','inner_values','inner','break','while','range','switch')),
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
    (Node 'branch' 'origin.flow.branch' 1040 330 @{ condition = $true }),
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
    (Link 'add' 'result' 'return' 'output_result'),
    (Link 'array' 'array' 'loop' 'array'),
    (Link 'loop' 'body' 'branch' 'exec')
)
$localFunctionVariables = @([ordered]@{ id = 'call_counter'; name = 'CallCounter'; type = 'integer'; defaultValue = 0; groupId = 'local'; description = '每次函数调用都必须重新初始化的局部变量' })
WriteArtifact 'functions/13_local_state_isolation.obpf' (NativeDocument '局部状态隔离' $localFunctionNodes $localFunctionLinks @(
    (New-GraphGroup 'local-main' '局部变量读写与函数返回' 35 50 1500 240 @('entry','sequence','get_before','add','set_after','return')),
    (New-GraphGroup 'local-branch' '函数内部数组循环与分支' 520 280 760 260 @('array','loop','branch'))
) $localFunctionVariables ([ordered]@{ functionId = 'functions/13_local_state_isolation.obpf'; functionCategory = '验证函数'; functionSignature = $localSignature }))

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
    (Node 'control_return' 'origin.result.append-string' 830 580),
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
    (Link 'control_call' 'exec' 'control_return' 'exec'),
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
    (New-GraphGroup 'control-call' '嵌套控制流函数调用' 480 530 650 180 @('control_call','control_return')),
    (New-GraphGroup 'local-isolation' '同输入连续调用：局部状态必须隔离' 480 740 1100 250 @('local_call_a','local_sequence','local_call_b','local_return_a','local_return_b'))
))

# 06: 定时器生命周期。正常入口创建/关闭，Timer 入口表示回调到达后的处理路径。
$timerNodes = @(
    (Node 'entry' 'origin.event.entry-two-integers' 80 130),
    (Node 'sequence' 'origin.flow.sequence' 300 130 @{} @{ label = '定时器生命周期'; dynamicOutputCount = 2 }),
    (Node 'params' 'origin.array.create-integer-new' 520 250 @{ items = @(7, 11, 13) }),
    (Node 'create_timer' 'origin.timer.create' 540 80 @{ milliseconds = 250 }),
    (Node 'debug_create' 'origin.debug.output' 780 80 @{ string = 'timer-created' }),
    (Node 'close_timer' 'origin.timer.close' 1020 80),
    (Node 'timer_entry' 'origin.event.timer' 80 520),
    (Node 'timer_debug' 'origin.debug.output' 320 520 @{ string = 'timer-fired' }),
    (Node 'timer_sequence' 'origin.flow.sequence' 560 520 @{} @{ label = 'Timer 结果序列'; dynamicOutputCount = 2 }),
    (Node 'timer_call' 'origin.function.call' 820 520 @{ input_seed = 11 } (FunctionCallProperties 'functions/13_local_state_isolation.obpf' '局部状态隔离' $localSignature)),
    (Node 'timer_result' 'origin.result.append-integer' 1110 520),
    (Node 'timer_text' 'origin.literal.string' 820 700 @{ value = 'timer-event-path' }),
    (Node 'timer_text_return' 'origin.result.append-string' 1110 700)
)
$timerLinks = @(
    (Link 'entry' 'exec' 'sequence' 'exec'),
    (Link 'sequence' 'then0' 'create_timer' 'exec'),
    (Link 'params' 'array' 'create_timer' 'params'),
    (Link 'create_timer' 'exec' 'debug_create' 'exec'),
    (Link 'sequence' 'then1' 'close_timer' 'exec'),
    (Link 'create_timer' 'timerId' 'close_timer' 'timerId'),
    (Link 'timer_entry' 'exec' 'timer_debug' 'exec'),
    (Link 'timer_debug' 'exec' 'timer_sequence' 'exec'),
    (Link 'timer_sequence' 'then0' 'timer_call' 'exec'),
    (Link 'timer_call' 'exec' 'timer_result' 'exec'),
    (Link 'timer_call' 'output_result' 'timer_result' 'value'),
    (Link 'timer_sequence' 'then1' 'timer_text_return' 'exec'),
    (Link 'timer_text' 'value' 'timer_text_return' 'value')
)
WriteArtifact '06_timer_lifecycle.obp' (NativeDocument '定时器生命周期' $timerNodes $timerLinks @(
    (New-GraphGroup 'timer-create-close' '常规入口：创建、观察与关闭定时器' 35 30 1250 330 @('entry','sequence','params','create_timer','debug_create','close_timer')),
    (New-GraphGroup 'timer-event' 'Timer 事件入口：回调处理和函数调用' 35 470 1320 320 @('timer_entry','timer_debug','timer_sequence','timer_call','timer_result','timer_text','timer_text_return'))
))

$coverage = [ordered]@{
    schemaVersion = 1
    description = '顶层系统节点的样本覆盖矩阵。visual 表示第 1 阶段人工检查；execution/async 留给后续阶段。'
    nodes = [ordered]@{
        'origin.event.entry-array' = @('03_array_data_lab.obp:visual')
        'origin.event.entry-two-integers' = @('02_control_flow_maze.obp:execution','04_deterministic_algorithm.obp:execution','05_function_orchestrator.obp:execution','06_timer_lifecycle.obp:async')
        'origin.event.timer' = @('06_timer_lifecycle.obp:async')
        'origin.debug.output' = @('01_legacy_all_nodes_showcase.vgf:visual','04_deterministic_algorithm.obp:visual','06_timer_lifecycle.obp:async')
        'origin.cast.integer-string' = @('03_array_data_lab.obp:visual','04_deterministic_algorithm.obp:visual')
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
        'origin.flow.sequence' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution','functions/12_nested_control_function.obpf:visual')
        'origin.flow.for-loop' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution')
        'origin.flow.branch' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution')
        'origin.flow.greater-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.less-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.equal-integer' = @('01_legacy_all_nodes_showcase.vgf:visual','02_control_flow_maze.obp:visual')
        'origin.flow.foreach-integer-array' = @('01_legacy_all_nodes_showcase.vgf:execution','02_control_flow_maze.obp:execution')
        'origin.flow.while' = @('02_control_flow_maze.obp:execution','functions/12_nested_control_function.obpf:visual')
        'origin.flow.for-loop-break' = @('02_control_flow_maze.obp:execution','functions/12_nested_control_function.obpf:visual')
        'origin.flow.foreach-array' = @('02_control_flow_maze.obp:visual')
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
        'origin.timer.create' = @('06_timer_lifecycle.obp:async')
        'origin.timer.close' = @('06_timer_lifecycle.obp:async')
        'origin.string.split' = @('03_array_data_lab.obp:visual')
        'origin.variable.get' = @('03_array_data_lab.obp:visual','functions/13_local_state_isolation.obpf:visual')
        'origin.variable.set' = @('03_array_data_lab.obp:visual','functions/13_local_state_isolation.obpf:visual')
        'origin.function.entry' = @('functions/10_score_kernel.obpf:visual','functions/11_array_fold_and_format.obpf:visual','functions/12_nested_control_function.obpf:visual','functions/13_local_state_isolation.obpf:visual')
        'origin.function.return' = @('functions/10_score_kernel.obpf:execution','functions/11_array_fold_and_format.obpf:execution','functions/12_nested_control_function.obpf:execution','functions/13_local_state_isolation.obpf:execution')
        'origin.function.call' = @('05_function_orchestrator.obp:execution','06_timer_lifecycle.obp:async','functions/11_array_fold_and_format.obpf:visual')
    }
}
WriteArtifact 'coverage.json' $coverage

$readme = @'
# 蓝图验证样本

这些文件是 OriginBlueprint 的人工可视化检查样本。它们只用于验证节点显示、端口、连线、分组、函数签名、变量和 legacy 导入形态；当前阶段不会修改引擎，也不会把这些样本作为自动化结果断言。

## 建议打开顺序

1. `01_legacy_all_nodes_showcase.vgf`：确认 legacy 节点、默认值和旧端口在新编辑器中的迁移显示。
2. `02_control_flow_maze.obp`：确认 Sequence、嵌套循环、动态分支、while、break 与任意数组循环的布局和连线。
3. `03_array_data_lab.obp`：确认数组控件、字符串控件、转换节点和局部变量节点。
4. `04_deterministic_algorithm.obp`：确认算术、浮点、比较、Branch、Range、Switch 和固定随机数的端口。
5. 打开 `functions/` 下四个 `.obpf`：确认函数入口/返回的参数名、类型和函数内变量显示。
6. `05_function_orchestrator.obp`：确认外部函数调用节点的输入输出端口，以及连续两次调用局部状态函数的可读性。
7. `06_timer_lifecycle.obp`：确认普通入口、Timer 入口、创建和关闭定时器节点的显示；本阶段不要实际依赖它的计时结果。

## 关键预期

- 函数入口、函数返回和函数调用节点的参数端口名称必须完整显示。
- 所有动态 Sequence、Range 与 Switch 的已连接分支端口必须可见。
- 图中每个分组标题应完整可读，节点不应重叠遮挡端口。
- `13_local_state_isolation.obpf` 的变量属于函数局部状态；`05_function_orchestrator.obp` 连续调用它两次，是后续隔离验证的样本入口。
- `coverage.json` 记录全部当前系统节点的样本位置和阶段覆盖范围。

## 后续阶段

第 2 阶段会为每个可执行样本编写独立 Go 参考实现并比较输出。第 3 阶段会在安全输入范围内生成带种子的随机参数。第 4、5 阶段仅在发现差异后总结并修复问题。
'@
WriteArtifact 'README.md' $readme

Write-Host "Generated verification blueprints at $OutputRoot"
