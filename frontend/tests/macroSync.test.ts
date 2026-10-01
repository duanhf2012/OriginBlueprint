import { describe, expect, it } from 'vitest'
import { syncMacroInstanceInDocument } from '../src/editor/macroSync'
import type { GraphDocument } from '../src/editor/document'

function macroDoc(): GraphDocument {
  return {
    schemaVersion: 1,
    graphName: '宏_受击',
    nodes: [
      { id: 'm-entry', typeId: 'origin.event.begin', position: { x: 0, y: 0 }, values: {} },
      { id: 'm-act', typeId: 'origin.action.print', position: { x: 200, y: 0 }, values: {} }
    ] as GraphDocument['nodes'],
    connections: [
      { source: 'm-entry', sourceOutput: 'exec', target: 'm-act', targetInput: 'exec' }
    ] as GraphDocument['connections'],
    groups: [], comments: [], macroInstances: [],
    variables: [{ id: 'mv1', name: '宏沉默_秒数', type: 'integer', defaultValue: 2, groupId: 'default' }],
    variableGroups: [], view: { x: 0, y: 0, zoom: 1 }
  } as GraphDocument
}

function targetDoc(instanceNodeIds: string[]): GraphDocument {
  return {
    schemaVersion: 1,
    graphName: 'test',
    nodes: [
      { id: 'ext-trigger', typeId: 'origin.event.begin', position: { x: -500, y: 0 }, values: {} },
      { id: instanceNodeIds[0], typeId: 'origin.event.begin', position: { x: 0, y: 0 }, values: {} },
      { id: instanceNodeIds[1], typeId: 'origin.action.print', position: { x: 200, y: 0 }, values: {} }
    ] as GraphDocument['nodes'],
    connections: [
      // 外部 → 实例（边界连线）：外部入口 exec → 实例第二个节点的 exec
      { source: 'ext-trigger', sourceOutput: 'exec', target: instanceNodeIds[1], targetInput: 'exec' },
      // 实例内部连线
      { source: instanceNodeIds[0], sourceOutput: 'exec', target: instanceNodeIds[1], targetInput: 'exec' }
    ] as GraphDocument['connections'],
    groups: [],
    comments: [{ id: 'frame1', text: '宏：旧名', x: 0, y: 0, width: 500, height: 200 }],
    macroInstances: [{ source: 'E:/ws/macros/宏_受击.obpm', commentId: 'frame1', nodeIds: [...instanceNodeIds] }],
    variables: [],
    variableGroups: [], view: { x: 0, y: 0, zoom: 1 }
  } as GraphDocument
}

describe('syncMacroInstanceInDocument（宏实例文档级同步）', () => {
  it('内容替换：旧节点消失、新节点 ID 全新、框标签刷新、实例记录更新', () => {
    const target = targetDoc(['old-1', 'old-2'])
    const macro = macroDoc()
    const ok = syncMacroInstanceInDocument(target, 'E:/ws/macros/宏_受击.obpm', macro, '宏_受击')
    expect(ok).toBe(true)
    const ids = target.nodes.map(node => node.id)
    expect(ids).not.toContain('old-1')
    expect(ids).not.toContain('old-2')
    expect(ids).toContain('ext-trigger')
    expect(target.nodes.filter(node => node.id !== 'ext-trigger')).toHaveLength(2)
    const record = target.macroInstances![0]!
    expect(record.commentId).toBe('frame1')
    expect(record.nodeIds).toHaveLength(2)
    expect(record.nodeIds.every(id => id !== 'old-1' && id !== 'old-2')).toBe(true)
    expect(target.comments![0]!.text).toBe('宏：宏_受击')
  })

  it('边界连线重接：外部连线保留并指向新实例对应节点，内部连线重建', () => {
    const target = targetDoc(['old-1', 'old-2'])
    const macro = macroDoc()
    syncMacroInstanceInDocument(target, 'E:/ws/macros/宏_受击.obpm', macro, '宏_受击')
    const record = target.macroInstances![0]!
    // 边界：ext-trigger exec → 实例第 2 个节点（源顺序 m-act）exec
    const boundary = target.connections!.find(c => c.source === 'ext-trigger')
    expect(boundary).toBeDefined()
    expect(boundary!.target).toBe(record.nodeIds[1])
    expect(boundary!.targetInput).toBe('exec')
    // 内部：m-entry exec → m-act exec
    const internal = target.connections!.find(c => c.source === record.nodeIds[0] && c.target === record.nodeIds[1])
    expect(internal).toBeDefined()
    // 旧实例相关连线全部清除
    expect(target.connections!.every(c => c.source !== 'old-1' && c.source !== 'old-2' && c.target !== 'old-1' && c.target !== 'old-2')).toBe(true)
  })

  it('变量合并：宏变量补建且 ID 冲突时生成新 ID；同名同类型复用目标已有变量', () => {
    const target = targetDoc(['old-1', 'old-2'])
    target.variables = [{ id: 'tv1', name: '宏沉默_秒数', type: 'integer', defaultValue: 99, groupId: 'default' }, { id: 'mv1', name: '别的', type: 'integer', defaultValue: 0, groupId: 'default' }]
    const macro = macroDoc()
    // 宏变量 mv1 与目标 tv1 同名同类型 → 复用；但目标已占用 mv1 这个 ID → 宏内引用应指向 tv1
    macro.nodes[1]!.properties = { variableId: 'mv1' }
    syncMacroInstanceInDocument(target, 'E:/ws/macros/宏_受击.obpm', macro, '宏_受击')
    expect(target.variables!.some(v => v.name === '宏沉默_秒数' && v.id === 'tv1')).toBe(true)
    expect(target.variables!.filter(v => v.name === '宏沉默_秒数')).toHaveLength(1)
    const record = target.macroInstances![0]!
    const actNode = target.nodes.find(node => node.id === record.nodeIds[1])
    expect(actNode!.properties!.variableId).toBe('tv1')
  })

  it('源宏无匹配实例时返回 false 不改动文档', () => {
    const target = targetDoc(['old-1', 'old-2'])
    const before = JSON.stringify(target)
    expect(syncMacroInstanceInDocument(target, 'E:/ws/不存在.obpm', macroDoc(), 'x')).toBe(false)
    expect(JSON.stringify(target)).toBe(before)
  })

  it('二次同步：更新后的实例可再次被同步（宏内容二次修改生效）', () => {
    const target = targetDoc(['old-1', 'old-2'])
    const macroV1 = macroDoc()
    syncMacroInstanceInDocument(target, 'E:/ws/macros/宏_受击.obpm', macroV1, '宏_受击')
    // 宏加了第三个节点
    const macroV2 = macroDoc()
    macroV2.nodes.push({ id: 'm-extra', typeId: 'origin.action.print', position: { x: 400, y: 0 }, values: {} } as GraphDocument['nodes'][number])
    macroV2.connections.push({ source: 'm-act', sourceOutput: 'exec', target: 'm-extra', targetInput: 'exec' } as GraphDocument['connections'][number])
    const ok = syncMacroInstanceInDocument(target, 'E:/ws/macros/宏_受击.obpm', macroV2, '宏_受击2')
    expect(ok).toBe(true)
    const record = target.macroInstances![0]!
    expect(record.nodeIds).toHaveLength(3)
    expect(target.comments![0]!.text).toBe('宏：宏_受击2')
    // 外部边界仍在
    const boundary = target.connections!.find(c => c.source === 'ext-trigger')
    expect(boundary!.target).toBe(record.nodeIds[1])
  })
})
