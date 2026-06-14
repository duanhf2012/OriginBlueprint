import { ClassicPreset } from 'rete'
import { BlueprintNode, type NodeKind } from './types'

export interface NodeDefinition {
  id: string
  title: string
  category: string
  kind: NodeKind
  create(): BlueprintNode
}

const sockets = {
  exec: new ClassicPreset.Socket('exec'),
  integer: new ClassicPreset.Socket('integer'),
  boolean: new ClassicPreset.Socket('boolean'),
  string: new ClassicPreset.Socket('string')
}

function input(socket: ClassicPreset.Socket, label: string, value?: string | number) {
  const port = new ClassicPreset.Input(socket, label)
  if (value !== undefined) {
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

export const nodeDefinitions: NodeDefinition[] = [
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

export function createNode(typeId: string) {
  const definition = nodeDefinitions.find(item => item.id === typeId)
  if (!definition) throw new Error(`Unknown node type: ${typeId}`)
  return definition.create()
}
