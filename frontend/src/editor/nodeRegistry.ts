import { ClassicPreset } from 'rete'
import { ArrayControl, BlueprintNode, FileControl, type NodeKind } from './types'
import type { GraphVariable, NodeProperties } from './document'

export interface NodeDefinition {
  id: string
  title: string
  category: string
  kind: NodeKind
  create(): BlueprintNode
}

type SocketType = keyof typeof sockets
interface PortSchema { key: string; label: string; type: SocketType; defaultValue?: unknown; arrayItemType?: 'string' | 'number'; fileMode?: 'open' | 'save' }
interface NodeSchema {
  id: string
  title: string
  category: string
  kind: NodeKind
  subtitle: string
  width?: number
  inputs?: PortSchema[]
  outputs?: PortSchema[]
  dynamicOutputs?: boolean
}

const sockets = {
  exec: new ClassicPreset.Socket('exec'),
  integer: new ClassicPreset.Socket('integer'),
  boolean: new ClassicPreset.Socket('boolean'),
  string: new ClassicPreset.Socket('string'),
  float: new ClassicPreset.Socket('float'),
  array: new ClassicPreset.Socket('array'),
  file: new ClassicPreset.Socket('file'),
  table: new ClassicPreset.Socket('table'),
  dictionary: new ClassicPreset.Socket('dictionary'),
  any: new ClassicPreset.Socket('any')
}

function input(socket: ClassicPreset.Socket, label: string, value?: unknown, arrayItemType: 'string' | 'number' = 'string', fileMode?: 'open' | 'save') {
  const port = new ClassicPreset.Input(socket, label)
  if (fileMode) {
    port.addControl(new FileControl(fileMode, String(value ?? '')))
  } else if (Array.isArray(value)) {
    port.addControl(new ArrayControl(arrayItemType, value))
  } else if (value !== undefined) {
    port.addControl(new ClassicPreset.InputControl(typeof value === 'number' ? 'number' : 'text', { initial: value }))
  }
  return port
}

function node(typeId: string, title: string, kind: NodeKind, subtitle: string, width: number) {
  const result = new BlueprintNode(title, kind, subtitle)
  result.typeId = typeId
  result.width = width
  return result
}

const coreDefinitions: NodeDefinition[] = [
  {
    id: 'origin.event.begin', title: 'Begin To Run', category: 'Action Default', kind: 'event',
    create() {
      const result = node(this.id, this.title, this.kind, 'Event', 210)
      result.addOutput('exec', new ClassicPreset.Output(sockets.exec, 'Begin'))
      return result
    }
  },
  {
    id: 'origin.flow.for-loop', title: 'For Loop', category: 'Basic Control', kind: 'flow',
    create() {
      const result = node(this.id, this.title, this.kind, 'Flow Control', 255)
      result.addInput('exec', input(sockets.exec, ''))
      result.addInput('start', input(sockets.integer, 'start', 0))
      result.addInput('end', input(sockets.integer, 'end', 10))
      result.addOutput('body', new ClassicPreset.Output(sockets.exec, 'Loop Body'))
      result.addOutput('index', new ClassicPreset.Output(sockets.integer, 'index'))
      result.addOutput('completed', new ClassicPreset.Output(sockets.exec, 'Completed'))
      return result
    }
  },
  {
    id: 'origin.flow.branch', title: 'Branch', category: 'Basic Control', kind: 'flow',
    create() {
      const result = node(this.id, this.title, this.kind, 'Flow Control', 235)
      result.addInput('exec', input(sockets.exec, ''))
      result.addInput('condition', input(sockets.boolean, 'Condition'))
      result.addOutput('true', new ClassicPreset.Output(sockets.exec, 'True'))
      result.addOutput('false', new ClassicPreset.Output(sockets.exec, 'False'))
      return result
    }
  },
  {
    id: 'origin.cast.integer-string', title: 'Integer To String', category: 'Casting', kind: 'function',
    create() {
      const result = node(this.id, this.title, this.kind, 'Casting', 225)
      result.addInput('value', input(sockets.integer, 'value', 0))
      result.addOutput('result', new ClassicPreset.Output(sockets.string, 'string'))
      return result
    }
  },
  {
    id: 'origin.action.print', title: 'Print To Console', category: 'Action Default', kind: 'event',
    create() {
      const result = node(this.id, this.title, this.kind, 'Action', 235)
      result.addInput('exec', input(sockets.exec, ''))
      result.addInput('value', input(sockets.string, 'str', 'Hello'))
      result.addOutput('exec', new ClassicPreset.Output(sockets.exec, ''))
      return result
    }
  }
]

function fromSchema(schema: NodeSchema): NodeDefinition {
  return {
    id: schema.id,
    title: schema.title,
    category: schema.category,
    kind: schema.kind,
    create() {
      const result = node(schema.id, schema.title, schema.kind, schema.subtitle, schema.width ?? 230)
      result.dynamicOutputs = schema.dynamicOutputs
      if (schema.dynamicOutputs) result.dynamicOutputCount = schema.outputs?.filter(port => port.key.startsWith('then')).length ?? 1
      for (const port of schema.inputs ?? []) result.addInput(port.key, input(sockets[port.type], port.label, port.defaultValue, port.arrayItemType, port.fileMode))
      for (const port of schema.outputs ?? []) result.addOutput(port.key, new ClassicPreset.Output(sockets[port.type], port.label))
      return result
    }
  }
}

const migratedSchemas: NodeSchema[] = [
  { id: 'origin.literal.string', title: 'String', category: 'Input', kind: 'function', subtitle: 'Literal', inputs: [{ key: 'value', label: 'value', type: 'string', defaultValue: '' }], outputs: [{ key: 'value', label: 'string', type: 'string' }] },
  { id: 'origin.math.add-float', title: 'Add (Float)', category: 'Math', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'float', defaultValue: 0 }, { key: 'b', label: 'B', type: 'float', defaultValue: 0 }], outputs: [{ key: 'result', label: 'result', type: 'float' }] },
  { id: 'origin.math.subtract-float', title: 'Subtract (Float)', category: 'Math', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'float', defaultValue: 0 }, { key: 'b', label: 'B', type: 'float', defaultValue: 0 }], outputs: [{ key: 'result', label: 'result', type: 'float' }] },
  { id: 'origin.math.multiply-float', title: 'Multiply (Float)', category: 'Math', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'float', defaultValue: 0 }, { key: 'b', label: 'B', type: 'float', defaultValue: 0 }], outputs: [{ key: 'result', label: 'result', type: 'float' }] },
  { id: 'origin.math.divide-float', title: 'Divide (Float)', category: 'Math', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'float', defaultValue: 0 }, { key: 'b', label: 'B', type: 'float', defaultValue: 1 }], outputs: [{ key: 'result', label: 'result', type: 'float' }] },
  { id: 'origin.compare.greater-integer', title: 'Greater (Integer)', category: 'Flow Control', kind: 'function', subtitle: 'Comparison', width: 255, inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'result', label: 'A > B', type: 'boolean' }, { key: 'a', label: 'A', type: 'integer' }, { key: 'b', label: 'B', type: 'integer' }] },
  { id: 'origin.flow.while', title: 'While', category: 'Flow Control', kind: 'flow', subtitle: 'Flow Control', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'condition', label: 'Condition', type: 'boolean' }], outputs: [{ key: 'body', label: 'Loop Body', type: 'exec' }, { key: 'completed', label: 'Completed', type: 'exec' }] },
  { id: 'origin.flow.for-loop-break', title: 'For Loop With Break', category: 'Flow Control', kind: 'flow', subtitle: 'Flow Control', width: 285, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'start', label: 'start', type: 'integer', defaultValue: 0 }, { key: 'end', label: 'end', type: 'integer', defaultValue: 10 }, { key: 'break', label: 'Break', type: 'exec' }], outputs: [{ key: 'body', label: 'Loop Body', type: 'exec' }, { key: 'index', label: 'index', type: 'integer' }, { key: 'completed', label: 'Completed', type: 'exec' }] },
  { id: 'origin.flow.foreach-array', title: 'For Each', category: 'Flow Control', kind: 'flow', subtitle: 'Flow Control', width: 255, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'array', label: 'array', type: 'array', defaultValue: [] }], outputs: [{ key: 'body', label: 'Loop Body', type: 'exec' }, { key: 'index', label: 'index', type: 'integer' }, { key: 'value', label: 'value', type: 'any' }, { key: 'completed', label: 'Completed', type: 'exec' }] },
  { id: 'origin.string.split', title: 'Split', category: 'String', kind: 'flow', subtitle: 'String', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'text', label: 'text', type: 'string', defaultValue: '' }, { key: 'delimiter', label: 'delimiter', type: 'string', defaultValue: ',' }], outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'array', label: 'array', type: 'array' }] },
  { id: 'origin.array.get-any', title: 'Get (Array)', category: 'Array', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: 'array', type: 'array', defaultValue: [] }, { key: 'index', label: 'index', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'value', label: 'value', type: 'any' }] },
  { id: 'origin.cast.any-string', title: 'Cast To String', category: 'Casting', kind: 'flow', subtitle: 'Casting', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'value', label: 'value', type: 'any' }], outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'valid', label: 'valid', type: 'boolean' }, { key: 'result', label: 'string', type: 'string' }] },
  { id: 'origin.dictionary.set', title: 'Set (Dict)', category: 'Dictionary', kind: 'flow', subtitle: 'Dictionary', width: 255, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'dictionary', label: 'dictionary', type: 'dictionary' }, { key: 'key', label: 'key', type: 'string', defaultValue: '' }, { key: 'value', label: 'value', type: 'any' }], outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'dictionary', label: 'dictionary', type: 'dictionary' }] },
  { id: 'origin.dictionary.size', title: 'Size (Dict)', category: 'Dictionary', kind: 'function', subtitle: 'Dictionary', inputs: [{ key: 'dictionary', label: 'dictionary', type: 'dictionary' }], outputs: [{ key: 'size', label: 'size', type: 'integer' }] },
  { id: 'origin.dictionary.keys', title: 'Keys (Dict)', category: 'Dictionary', kind: 'function', subtitle: 'Dictionary', inputs: [{ key: 'dictionary', label: 'dictionary', type: 'dictionary' }], outputs: [{ key: 'keys', label: 'keys', type: 'array' }] },
  { id: 'origin.cast.float-string', title: 'Float To String', category: 'Casting', kind: 'function', subtitle: 'Casting', inputs: [{ key: 'value', label: 'value', type: 'float', defaultValue: 0 }], outputs: [{ key: 'result', label: 'string', type: 'string' }] },
  { id: 'origin.math.add-integer', title: '加 (Integer)', category: '运算', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'result', label: '结果', type: 'integer' }] },
  { id: 'origin.math.subtract-integer', title: '减 (Integer)', category: '运算', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }, { key: 'absolute', label: 'abs', type: 'boolean' }], outputs: [{ key: 'result', label: '结果', type: 'integer' }] },
  { id: 'origin.math.multiply-integer', title: '乘 (Integer)', category: '运算', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'result', label: '结果', type: 'integer' }] },
  { id: 'origin.math.divide-integer', title: '除 (Integer)', category: '运算', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 1 }, { key: 'round', label: '四舍五入', type: 'boolean' }], outputs: [{ key: 'result', label: '结果', type: 'integer' }] },
  { id: 'origin.math.modulo-integer', title: '取模 (Integer)', category: '运算', kind: 'function', subtitle: 'Math', inputs: [{ key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 1 }], outputs: [{ key: 'result', label: '结果', type: 'integer' }] },
  { id: 'origin.math.random-integer', title: '范围随机 [0,99]', category: '运算', kind: 'function', subtitle: 'Math', width: 245, inputs: [{ key: 'seed', label: '随机种子', type: 'integer', defaultValue: 0 }, { key: 'min', label: '最小值', type: 'integer', defaultValue: 0 }, { key: 'max', label: '最大值', type: 'integer', defaultValue: 99 }], outputs: [{ key: 'result', label: '随机数', type: 'integer' }] },
  { id: 'origin.flow.sequence', title: '序列', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', dynamicOutputs: true, inputs: [{ key: 'exec', label: '', type: 'exec' }], outputs: [{ key: 'then0', label: 'Then 0', type: 'exec' }, { key: 'then1', label: 'Then 1', type: 'exec' }, { key: 'then2', label: 'Then 2', type: 'exec' }] },
  { id: 'origin.flow.greater-integer', title: '大于 (Integer) >', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', width: 245, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'orEqual', label: '>=', type: 'boolean' }, { key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'false', label: '假', type: 'exec' }, { key: 'true', label: '真', type: 'exec' }] },
  { id: 'origin.flow.less-integer', title: '小于 (Integer) <', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', width: 245, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'orEqual', label: '<=', type: 'boolean' }, { key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'false', label: '假', type: 'exec' }, { key: 'true', label: '真', type: 'exec' }] },
  { id: 'origin.flow.equal-integer', title: '等于 (Integer) ==', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', width: 245, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'a', label: 'A', type: 'integer', defaultValue: 0 }, { key: 'b', label: 'B', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'false', label: '假', type: 'exec' }, { key: 'true', label: '真', type: 'exec' }] },
  { id: 'origin.array.get-integer', title: '获取数组值 (Integer)', category: '基础', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: '数组', type: 'array', defaultValue: [], arrayItemType: 'number' }, { key: 'index', label: '索引', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'value', label: '值', type: 'integer' }] },
  { id: 'origin.array.get-string', title: '获取数组值 (String)', category: '基础', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: '数组', type: 'array', defaultValue: [], arrayItemType: 'string' }, { key: 'index', label: '索引', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'value', label: '值', type: 'string' }] },
  { id: 'origin.array.length', title: '获取数组长度', category: '基础', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: '数组', type: 'array', defaultValue: [] }], outputs: [{ key: 'length', label: '长度', type: 'integer' }] },
  { id: 'origin.array.create-integer', title: '创建整型数组', category: '基础', kind: 'function', subtitle: 'Array', width: 250, inputs: [{ key: 'items', label: '', type: 'array', defaultValue: [], arrayItemType: 'number' }], outputs: [{ key: 'array', label: '数组', type: 'array' }] },
  { id: 'origin.array.create-string', title: '创建字符串数组', category: '基础', kind: 'function', subtitle: 'Array', width: 250, inputs: [{ key: 'items', label: '', type: 'array', defaultValue: [], arrayItemType: 'string' }], outputs: [{ key: 'array', label: '数组', type: 'array' }] },
  { id: 'origin.array.append-string', title: '数组追加字符串', category: '基础', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: '数组', type: 'array', defaultValue: [] }, { key: 'value', label: '字符串', type: 'string', defaultValue: '' }], outputs: [{ key: 'array', label: '数组', type: 'array' }] },
  { id: 'origin.array.append-integer', title: '数组追加整型', category: '基础', kind: 'function', subtitle: 'Array', inputs: [{ key: 'array', label: '数组', type: 'array', defaultValue: [], arrayItemType: 'number' }, { key: 'value', label: '数值', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'array', label: '数组', type: 'array' }] },
  { id: 'origin.result.append-integer', title: '追加返回结果 (Integer)', category: '基础', kind: 'flow', subtitle: 'Result', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'value', label: '返回值', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'exec', label: '', type: 'exec' }] },
  { id: 'origin.result.append-string', title: '追加返回结果 (String)', category: '基础', kind: 'flow', subtitle: 'Result', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'value', label: '返回值', type: 'string', defaultValue: '' }], outputs: [{ key: 'exec', label: '', type: 'exec' }] },
  { id: 'origin.event.entry-array', title: '执行入口 (数组)', category: '入口', kind: 'event', subtitle: 'Entry', outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'objectId', label: '对象ID', type: 'integer' }, { key: 'params', label: 'param数组', type: 'array' }] },
  { id: 'origin.event.entry-two-integers', title: '执行入口 (2参数)', category: '入口', kind: 'event', subtitle: 'Entry', outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'objectId', label: '对象ID', type: 'integer' }, { key: 'param1', label: '参数1', type: 'integer' }, { key: 'param2', label: '参数2', type: 'integer' }] },
  { id: 'origin.event.timer', title: 'Timer事件入口', category: '入口', kind: 'event', subtitle: 'Entry', outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'timerId', label: '定时器ID', type: 'integer' }, { key: 'params', label: '附加参数', type: 'array' }] },
  { id: 'origin.timer.create', title: '创建定时器', category: '基础', kind: 'flow', subtitle: 'Timer', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'milliseconds', label: '时间(毫秒)', type: 'integer', defaultValue: 1000 }, { key: 'params', label: '附加参数', type: 'array', defaultValue: [] }], outputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'timerId', label: '定时器ID', type: 'integer' }] },
  { id: 'origin.timer.close', title: '关闭定时器', category: '基础', kind: 'flow', subtitle: 'Timer', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'timerId', label: '定时器ID', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'exec', label: '', type: 'exec' }] },
  { id: 'origin.flow.foreach-integer-array', title: 'For循环 (整型数组)', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', width: 260, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'array', label: '整型数组', type: 'array', defaultValue: [], arrayItemType: 'number' }], outputs: [{ key: 'body', label: 'Loop Body', type: 'exec' }, { key: 'completed', label: 'Completed', type: 'exec' }, { key: 'index', label: '数组下标', type: 'integer' }, { key: 'value', label: '数组元素', type: 'integer' }] },
  { id: 'origin.flow.probability', title: '概率判断 (万分比)', category: '流程控制', kind: 'flow', subtitle: 'Flow Control', inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'probability', label: '概率', type: 'integer', defaultValue: 0 }], outputs: [{ key: 'miss', label: '未命中', type: 'exec' }, { key: 'hit', label: '命中', type: 'exec' }] },
  { id: 'origin.flow.range-compare', title: '范围比较 <=', category: '流程控制', kind: 'flow', subtitle: 'Multi Branch', width: 260, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'value', label: '值', type: 'integer', defaultValue: 0 }, { key: 'ranges', label: '范围', type: 'array', defaultValue: [], arrayItemType: 'number' }], outputs: [{ key: 'otherwise', label: '否则', type: 'exec' }, { key: 'case0', label: 'Case 0', type: 'exec' }, { key: 'case1', label: 'Case 1', type: 'exec' }, { key: 'case2', label: 'Case 2', type: 'exec' }, { key: 'case3', label: 'Case 3', type: 'exec' }, { key: 'case4', label: 'Case 4', type: 'exec' }] },
  { id: 'origin.flow.equal-switch', title: '等于分支 ==', category: '流程控制', kind: 'flow', subtitle: 'Multi Branch', width: 260, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'value', label: '值', type: 'integer', defaultValue: 0 }, { key: 'cases', label: '匹配值', type: 'array', defaultValue: [], arrayItemType: 'number' }], outputs: [{ key: 'otherwise', label: '否则', type: 'exec' }, { key: 'case0', label: 'Case 0', type: 'exec' }, { key: 'case1', label: 'Case 1', type: 'exec' }, { key: 'case2', label: 'Case 2', type: 'exec' }, { key: 'case3', label: 'Case 3', type: 'exec' }, { key: 'case4', label: 'Case 4', type: 'exec' }] },
  { id: 'origin.debug.output', title: '打印调试信息', category: '测试', kind: 'flow', subtitle: 'Debug', width: 260, inputs: [{ key: 'exec', label: '', type: 'exec' }, { key: 'integer', label: '要打印的值1', type: 'integer', defaultValue: 0 }, { key: 'string', label: '要打印的值2', type: 'string', defaultValue: '' }, { key: 'array', label: '要打印的值3', type: 'array', defaultValue: [] }], outputs: [{ key: 'exec', label: '', type: 'exec' }] }
]

const allNodeDefinitions: NodeDefinition[] = [...coreDefinitions, ...migratedSchemas.map(fromSchema)]
const hiddenNodeTypes = new Set(['origin.event.timer', 'origin.timer.create', 'origin.timer.close'])

// Hidden types remain constructible so legacy documents can still be opened.
export const nodeDefinitions: NodeDefinition[] = allNodeDefinitions.filter(item => !hiddenNodeTypes.has(item.id))

export function createNode(typeId: string) {
  const definition = allNodeDefinitions.find(item => item.id === typeId)
  if (!definition) throw new Error(`Unknown node type: ${typeId}`)
  return definition.create()
}

function variableSocket(type: GraphVariable['type']) {
  return sockets[type]
}

export function createVariableNode(variable: GraphVariable, access: 'get' | 'set') {
  const typeId = `origin.variable.${access}`
  const title = `${access === 'get' ? 'Get' : 'Set'} ${variable.name}`
  const result = node(typeId, title, 'variable', `${variable.type} variable`, 220)
  result.variableId = variable.id
  result.variableAccess = access
  result.compact = access === 'get'
  const socket = variableSocket(variable.type)
  if (access === 'get') {
    result.addOutput('value', new ClassicPreset.Output(socket, variable.name))
  } else {
    result.addInput('exec', input(sockets.exec, ''))
    result.addInput('value', input(socket, variable.name, variable.defaultValue, variable.type === 'array' ? 'string' : 'string'))
    result.addOutput('exec', new ClassicPreset.Output(sockets.exec, ''))
    result.addOutput('value', new ClassicPreset.Output(socket, variable.name))
  }
  return result
}

export function createLegacyNode(properties: NodeProperties) {
  const result = node('origin.legacy.placeholder', properties.label || properties.legacyClass || 'Legacy Node', 'function', `Legacy: ${properties.legacyModule || 'unknown'}`, 245)
  result.legacyClass = properties.legacyClass
  result.legacyModule = properties.legacyModule
  result.legacyInputs = properties.legacyInputs?.map(port => ({ ...port })) ?? []
  result.legacyOutputs = properties.legacyOutputs?.map(port => ({ ...port })) ?? []
  for (const port of result.legacyInputs) result.addInput(port.key, input(sockets[port.type as SocketType] ?? sockets.any, port.label, port.type === 'exec' ? undefined : ''))
  for (const port of result.legacyOutputs) result.addOutput(port.key, new ClassicPreset.Output(sockets[port.type as SocketType] ?? sockets.any, port.label))
  return result
}
