import { ClassicPreset, GetSchemes } from 'rete'
import type { EntryPortBinding } from './implicitEntryLinks'
import type { SocketThemeName } from './socketTheme'

export type NodeKind = 'event' | 'flow' | 'function' | 'variable'
export interface PortVisualState { connected: boolean; filled: boolean; entryBinding?: EntryPortBinding }
export interface NodePortVisualStates {
  inputs: Record<string, PortVisualState>
  outputs: Record<string, PortVisualState>
}

export interface DynamicBranchConfig {
  controlInput: string
  defaultOutput: string
  outputPrefix: string
  outputStartIndex: number
  maxBranches: number
  hiddenOutputKeys?: string[]
}

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
  dynamicBranch?: DynamicBranchConfig
  executionState?: 'idle' | 'running' | 'completed' | 'error'
  legacyClass?: string
  legacyModule?: string
  legacyInputs?: Array<{ key: string; label: string; type: string }>
  legacyOutputs?: Array<{ key: string; label: string; type: string }>
  portStates?: NodePortVisualStates

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

export type BlueprintConnection = ClassicPreset.Connection<BlueprintNode, BlueprintNode> & { selected?: boolean; socketType?: SocketThemeName; hidden?: boolean }
export type Schemes = GetSchemes<BlueprintNode, BlueprintConnection>
