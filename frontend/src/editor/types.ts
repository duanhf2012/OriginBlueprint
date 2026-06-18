import { ClassicPreset, GetSchemes } from 'rete'
import type { SocketThemeName } from './socketTheme'

export type NodeKind = 'event' | 'flow' | 'function' | 'variable'

export class BlueprintNode extends ClassicPreset.Node {
  typeId?: string
  kind?: NodeKind
  subtitle?: string
  width?: number
  compact?: boolean
  variableId?: string
  variableAccess?: 'get' | 'set'
  dynamicOutputs?: boolean
  dynamicOutputCount?: number
  executionState?: 'idle' | 'running' | 'completed' | 'error'
  legacyClass?: string
  legacyModule?: string
  legacyInputs?: Array<{ key: string; label: string; type: string }>
  legacyOutputs?: Array<{ key: string; label: string; type: string }>

  constructor(label: string, kind: NodeKind = 'function', subtitle?: string) {
    super(label)
    this.kind = kind
    this.subtitle = subtitle
  }
}

export class ArrayControl extends ClassicPreset.Control {
  value: Array<string | number>
  itemType: 'string' | 'number'

  constructor(itemType: 'string' | 'number', initial: Array<string | number> = []) {
    super()
    this.itemType = itemType
    this.value = [...initial]
  }

  setValue(value?: unknown) {
    this.value = Array.isArray(value) ? [...value] : []
  }
}

export class FileControl extends ClassicPreset.Control {
  value: string
  mode: 'open' | 'save'

  constructor(mode: 'open' | 'save', initial = '') {
    super()
    this.mode = mode
    this.value = initial
  }

  setValue(value?: unknown) {
    this.value = String(value ?? '')
  }
}

export type BlueprintConnection = ClassicPreset.Connection<BlueprintNode, BlueprintNode> & { selected?: boolean; socketType?: SocketThemeName }
export type Schemes = GetSchemes<BlueprintNode, BlueprintConnection>
