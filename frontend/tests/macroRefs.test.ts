import { describe, expect, it } from 'vitest'
import { splitSnapshotForPersistence, snapshotHasMacroRef, workspaceRelativeHint } from '../src/editor/macroRefs'
import type { GraphSnapshot, NodeSnapshot } from '../src/editor/document'

function node(id: string, x = 0, y = 0): NodeSnapshot {
  return { id, typeId: 'origin.math.add-integer', position: { x, y }, values: {} }
}

function connection(source: string, sourceOutput: string, target: string, targetInput: string) {
  return { source, sourceOutput, target, targetInput }
}

function snapshot(macroId: string, mirrorIds: string[]): GraphSnapshot {
  return {
    nodes: [
      node('host-a'),
      node('host-b'),
      ...mirrorIds.filter(Boolean).map(id => ({ ...node(id), mirrorMacro: true }))
    ],
    connections: [
      connection('host-a', 'result', 'host-b', 'a'),
      connection('host-a', 'exec', mirrorIds[2] ?? '', 'exec'),        // 外部 → 宏（intoMacro）
      connection(mirrorIds[1] ?? '', 'result', 'host-b', 'integer'),  // 宏 → 外部
      connection(mirrorIds[0] ?? '', 'exec', mirrorIds[1] ?? '', 'exec') // 宏内部连线（不持久化）
    ],
    groups: [{ id: 'g1', title: 'G', x: 0, y: 0, width: 10, height: 10, nodeIds: ['host-a', mirrorIds[0] ?? ''] }],
    comments: [],
    macroRefs: [{ macroId, pathHint: 'macros/a.obpm', commentId: 'c1', frame: { x: 1, y: 2, width: 100, height: 80 }, mirrorIds: [...mirrorIds] }]
  }
}

describe('splitSnapshotForPersistence（宏引用持久化拆分）', () => {
  it('镜像节点剔除、普通连线保留、boundary 按方向生成', () => {
    const data = snapshot('m1', ['mir0', 'mir1', 'mir2'])
    const persisted = splitSnapshotForPersistence(data)

    expect(persisted.nodes.map(item => item.id)).toEqual(['host-a', 'host-b'])
    expect(persisted.connections).toEqual([connection('host-a', 'result', 'host-b', 'a')])

    expect(persisted.macroRefs).toHaveLength(1)
    const ref = persisted.macroRefs[0]!
    expect(ref.macroId).toBe('m1')
    // 引用只落宏 ID + 几何 + 边界：pathHint/commentId（运行时字段）不出现在持久化输出。
    expect(ref.pathHint).toBeUndefined()
    expect(ref.commentId).toBeUndefined()
    expect(ref.frame).toEqual({ x: 1, y: 2, width: 100, height: 80 })
    expect(ref.boundary).toEqual([
      { externalNode: 'host-a', externalPort: 'exec', macroNodeIndex: 2, macroPort: 'exec', intoMacro: true },
      { externalNode: 'host-b', externalPort: 'integer', macroNodeIndex: 1, macroPort: 'result', intoMacro: false }
    ])
    // 宏内部连线（mir0→mir1）不进 boundary 也不进 connections：展开时按源重建。
    expect(ref.boundary!.length).toBe(2)
  })

  it('宏内被跳过节点的下标（mirrorIds 中的空位）不产生 boundary', () => {
    const data: GraphSnapshot = {
      nodes: [node('host-a'), node('host-b'), { ...node('mir0'), mirrorMacro: true }, { ...node('mir2'), mirrorMacro: true }],
      connections: [
        connection('host-a', 'exec', 'mir2', 'exec'),
        connection('host-a', 'exec', 'mir0', 'exec'),
        connection('mir0', 'exec', 'mir2', 'exec')
      ],
      groups: [],
      comments: [],
      macroRefs: [{ macroId: 'm1', mirrorIds: ['mir0', '', 'mir2'] }]
    }
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.nodes.map(item => item.id)).toEqual(['host-a', 'host-b'])
    expect(persisted.connections).toEqual([])
    const boundary = persisted.macroRefs[0]!.boundary!
    expect(boundary.map(item => item.macroNodeIndex).sort()).toEqual([0, 2])
    expect(boundary.every(item => item.macroNodeIndex !== 1)).toBe(true)
  })

  it('分组的 nodeIds 剔除镜像 id（镜像 id 每次展开重新生成）', () => {
    const data = snapshot('m1', ['mir0', 'mir1', 'mir2'])
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.groups[0]!.nodeIds).toEqual(['host-a'])
  })

  it('mirrorMacro 标记但不在 mirrorIds 里的孤儿节点也剔除（防御）', () => {
    const data: GraphSnapshot = {
      nodes: [node('host-a'), { ...node('orphan'), mirrorMacro: true }],
      connections: [],
      groups: [],
      comments: [],
      macroRefs: []
    }
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.nodes.map(item => item.id)).toEqual(['host-a'])
  })

  it('宏尚未展开（无镜像）时保留原有 boundary 记录，避免加载后立即保存丢数据', () => {
    const data: GraphSnapshot = {
      nodes: [node('host-a')],
      connections: [],
      groups: [],
      comments: [],
      macroRefs: [{
        macroId: 'm1',
        pathHint: 'macros/a.obpm',
        commentId: 'c1',
        frame: { x: 0, y: 0, width: 100, height: 80 },
        boundary: [{ externalNode: 'host-a', externalPort: 'exec', macroNodeIndex: 2, macroPort: 'exec', intoMacro: true }],
        mirrorIds: []
      }]
    }
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.macroRefs[0]!.boundary).toEqual([{ externalNode: 'host-a', externalPort: 'exec', macroNodeIndex: 2, macroPort: 'exec', intoMacro: true }])
  })

  it('宏框注释不入盘：持久化 comments 剔除宏框，几何只存在于 macroRefs.frame', () => {
    const data: GraphSnapshot = {
      nodes: [node('host-a')],
      connections: [],
      groups: [],
      comments: [
        { id: 'note-1', text: '用户自己的便签', x: 0, y: 0, width: 100, height: 50 },
        { id: 'frame-1', text: '宏：旧名字（过期也不落盘）', x: 1, y: 2, width: 300, height: 200 }
      ],
      macroRefs: [{ macroId: 'm1', commentId: 'frame-1', mirrorIds: ['mir0'], frame: { x: 1, y: 2, width: 300, height: 200 } }]
    }
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.comments.map(item => item.id)).toEqual(['note-1'])
    expect(persisted.macroRefs[0]!.frame).toEqual({ x: 1, y: 2, width: 300, height: 200 })
  })

  it('同一宏多次引用：boundary 归属各自的引用记录', () => {
    const data: GraphSnapshot = {
      nodes: [node('host-a'), { ...node('m-a'), mirrorMacro: true }, { ...node('m-b'), mirrorMacro: true }],
      connections: [
        connection('host-a', 'exec', 'm-a', 'exec'),
        connection('host-a', 'exec', 'm-b', 'exec')
      ],
      groups: [],
      comments: [],
      macroRefs: [
        { macroId: 'm1', mirrorIds: ['m-a'] },
        { macroId: 'm1', mirrorIds: ['m-b'] }
      ]
    }
    const persisted = splitSnapshotForPersistence(data)
    expect(persisted.macroRefs[0]!.boundary).toEqual([{ externalNode: 'host-a', externalPort: 'exec', macroNodeIndex: 0, macroPort: 'exec', intoMacro: true }])
    expect(persisted.macroRefs[1]!.boundary).toEqual([{ externalNode: 'host-a', externalPort: 'exec', macroNodeIndex: 0, macroPort: 'exec', intoMacro: true }])
  })
})


describe('workspaceRelativeHint（pathHint 归一化）', () => {
  const root = 'E:\\ws\\project'

  it('工作区内绝对路径转相对路径（正斜杠）', () => {
    expect(workspaceRelativeHint('E:\\ws\\project\\vgf\\宏_A.obpm', root)).toBe('vgf/宏_A.obpm')
    expect(workspaceRelativeHint('E:/ws/project/vgf/宏_A.obpm', root)).toBe('vgf/宏_A.obpm')
  })

  it('工作区外路径退化为文件名（不落绝对路径）', () => {
    expect(workspaceRelativeHint('D:\\elsewhere\\宏_B.obpm', root)).toBe('宏_B.obpm')
    expect(workspaceRelativeHint('', root)).toBe('')
  })
})

describe('snapshotHasMacroRef', () => {
  it('按稳定身份判定是否已引用', () => {
    const data: GraphSnapshot = { nodes: [], connections: [], groups: [], comments: [], macroRefs: [{ macroId: 'm1' }] }
    expect(snapshotHasMacroRef(data, 'm1')).toBe(true)
    expect(snapshotHasMacroRef(data, 'other')).toBe(false)
    expect(snapshotHasMacroRef({ nodes: [], connections: [], groups: [], comments: [] }, 'm1')).toBe(false)
  })
})
