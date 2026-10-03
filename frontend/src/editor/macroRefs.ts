import type { ConnectionSnapshot, GraphSnapshot, GraphVariable, MacroRefBoundary, MacroRefSnapshot, NodeSnapshot } from './document'

// 宏引用（v2 引用模型）：引用图不保存宏内容，只保存 macroId + 边界连线；编辑器把宏源
// 展开为只读"镜像节点"画在宏框内，保存时镜像节点与跨宏连线被拆回 macroRefs.boundary。
// 本模块承载可纯单测的拆分/合并规则。

/** 宏展开载荷：App.vue 从 .obpm 源文件解析后交给编辑器展开/热更。 */
export interface MacroMirrorPayload {
  macroId: string
  label: string
  pathHint?: string
  /** 宏文档节点/连线（镜像与内部连线的来源）。 */
  snapshot: Pick<GraphSnapshot, 'nodes' | 'connections'>
  /** 宏文档自己的变量表：镜像变量节点按它解析显示（运行时由引擎按名合并，不落引用图）。 */
  variables?: GraphVariable[]
}

export interface MacroMirrorSite {
  refIndex: number
  sourceIndex: number
}

export interface PersistedGraphSplit {
  nodes: NodeSnapshot[]
  connections: ConnectionSnapshot[]
  macroRefs: MacroRefSnapshot[]
  groups: GraphSnapshot['groups']
  comments: GraphSnapshot['comments']
}

/**
 * 把编辑器快照拆成持久化形态：
 * - 镜像节点从 nodes 剔除（宏内容以 .obpm 源为准，不落引用图）；
 * - 恰好一端是镜像的连线转成所属 macroRefs 的 boundary 记录
 *   （镜像端按宏源节点顺序记录 macroNodeIndex，外部端记录节点与端口；intoMacro=外部→宏）；
 * - 两端都是镜像（宏内部连线或跨宏连线）不持久化：前者在展开时按源重建，后者无运行时语义；
 * - 宏尚未展开（无镜像，如加载后立刻保存）时保留原有 boundary 记录，避免数据丢失；
 * - 引用只落 macroId + 外框几何 + 边界：宏名/路径等一切可从宏源推导的信息不落盘
 *   （frame 文本由加载/热更时从宏源实时渲染，改名永不过期）；
 * - 宏框注释（编辑器用注释机制承载拖拽/缩放）是运行时对象，不进 comments；
 * - 分组的 nodeIds 剔除镜像 id（镜像 id 每次展开重新生成，落盘必然悬空）。
 */
export function splitSnapshotForPersistence(data: GraphSnapshot): PersistedGraphSplit {
  const refs = data.macroRefs ?? []
  const mirrorSiteByNodeId = new Map<string, MacroMirrorSite>()
  for (const [refIndex, ref] of refs.entries()) {
    for (const [sourceIndex, nodeId] of (ref.mirrorIds ?? []).entries()) {
      if (nodeId) mirrorSiteByNodeId.set(nodeId, { refIndex, sourceIndex })
    }
  }
  const nodes = data.nodes.filter(node => !mirrorSiteByNodeId.has(node.id) && !node.mirrorMacro)
  const boundaries: MacroRefBoundary[][] = refs.map(ref => (ref.mirrorIds ?? []).some(Boolean) ? [] : (ref.boundary ?? []).map(item => ({ ...item })))
  const connections: ConnectionSnapshot[] = []
  for (const connection of data.connections) {
    const sourceSite = mirrorSiteByNodeId.get(connection.source)
    const targetSite = mirrorSiteByNodeId.get(connection.target)
    if (sourceSite && targetSite) continue
    if (sourceSite) {
      boundaries[sourceSite.refIndex]!.push({
        externalNode: connection.target,
        externalPort: String(connection.targetInput),
        macroNodeIndex: sourceSite.sourceIndex,
        macroPort: String(connection.sourceOutput),
        intoMacro: false,
      })
      continue
    }
    if (targetSite) {
      boundaries[targetSite.refIndex]!.push({
        externalNode: connection.source,
        externalPort: String(connection.sourceOutput),
        macroNodeIndex: targetSite.sourceIndex,
        macroPort: String(connection.targetInput),
        intoMacro: true,
      })
      continue
    }
    connections.push(connection)
  }
  const macroRefs: MacroRefSnapshot[] = refs.map((ref, refIndex) => ({
    macroId: ref.macroId,
    frame: ref.frame,
    boundary: boundaries[refIndex],
  }))
  const groups = data.groups.map(group => ({ ...group, nodeIds: group.nodeIds.filter(id => !mirrorSiteByNodeId.has(id)) }))
  // 宏框注释是运行时对象（几何已序列化进 macroRefs.frame，文本由宏源派生），不进 comments。
  const frameCommentIds = new Set(refs.map(ref => ref.commentId).filter(Boolean))
  const comments = data.comments.filter(comment => !frameCommentIds.has(comment.id))
  return { nodes, connections, macroRefs, groups, comments }
}

/** 该宏是否已被当前图引用（按稳定身份 macroId 判定）。 */
export function snapshotHasMacroRef(data: GraphSnapshot, macroId: string) {
  return (data.macroRefs ?? []).some(ref => ref.macroId === macroId)
}

/**
 * pathHint 归一化为工作区相对路径（正斜杠）：仅作人读提示，解析永远按 macroId。
 * 不在工作区下的路径退化为文件名——绝对路径不可移植，也泄漏本机目录结构。
 */
export function workspaceRelativeHint(path: string, workspaceRoot: string): string {
  const clean = String(path ?? '').trim()
  if (!clean) return ''
  const normalize = (value: string) => value.replace(/[\\/]+/g, '/').replace(/\/+$/, '').toLowerCase()
  const root = normalize(workspaceRoot)
  const normalizedPath = clean.replace(/[\\/]+/g, '/')
  if (root && normalize(normalizedPath).startsWith(`${root}/`)) {
    return normalizedPath.slice(root.length + 1)
  }
  if (root && normalize(normalizedPath) === root) return ''
  const filename = normalizedPath.split('/').pop() ?? normalizedPath
  return filename
}
