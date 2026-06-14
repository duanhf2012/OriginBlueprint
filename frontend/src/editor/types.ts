import { ClassicPreset, GetSchemes } from 'rete'

export type NodeKind = 'event' | 'flow' | 'function' | 'variable'

export class BlueprintNode extends ClassicPreset.Node {
  typeId?: string
  kind?: NodeKind
  subtitle?: string
  width?: number
  compact?: boolean

  constructor(label: string, kind: NodeKind = 'function', subtitle?: string) {
    super(label)
    this.kind = kind
    this.subtitle = subtitle
  }
}

export type BlueprintConnection = ClassicPreset.Connection<BlueprintNode, BlueprintNode>
export type Schemes = GetSchemes<BlueprintNode, BlueprintConnection>
