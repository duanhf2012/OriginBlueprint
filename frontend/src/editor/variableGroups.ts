import { variableScope, type GraphVariable, type GraphVariableGroup, type VariableScope } from './document'

export interface VariableGroupUsage {
  localCount: number
  globalCount: number
  totalCount: number
}

export interface NormalizedVariableGroups {
  groups: GraphVariableGroup[]
  resolveGroupId(requestedGroupId: string, legacyGroupName: string, scope: VariableScope): string
}

export function variableGroupScope(group: Pick<GraphVariableGroup, 'id' | 'scope'>): VariableScope | null {
  if (group.id === 'default') return null
  return group.scope === 'instance' ? 'instance' : 'execution'
}

export function variableGroupsForScope(groups: GraphVariableGroup[], scope: VariableScope) {
  return groups.filter(group => group.id === 'default' || variableGroupScope(group) === scope)
}

export function variableGroupUsage(variables: GraphVariable[], groupId: string): VariableGroupUsage {
  let localCount = 0
  let globalCount = 0
  for (const variable of variables) {
    if (variable.groupId !== groupId) continue
    if (variableScope(variable) === 'instance') globalCount++
    else localCount++
  }
  return { localCount, globalCount, totalCount: localCount + globalCount }
}

export function variableGroupRemovalMessage(groupName: string, scope: VariableScope, count: number) {
  const scopeName = scope === 'instance' ? '全局' : '局部'
  return `删除${scopeName}分组“${groupName}”？其中 ${count} 个${scopeName}变量将移动到 Default。`
}

export function variableGroupNameExists(groups: GraphVariableGroup[], name: string, scope: VariableScope, exceptId = '') {
  const key = name.trim().toLowerCase()
  return variableGroupsForScope(groups, scope).some(group => group.id !== exceptId && group.name.trim().toLowerCase() === key)
}

export function moveVariablesToDefaultGroup(variables: GraphVariable[], groupId: string) {
  for (const variable of variables) if (variable.groupId === groupId) variable.groupId = 'default'
}

export function matchingVariableGroupId(groups: GraphVariableGroup[], currentGroupId: string, targetScope: VariableScope) {
  if (!currentGroupId || currentGroupId === 'default') return 'default'
  const current = groups.find(group => group.id === currentGroupId)
  if (!current) return 'default'
  const name = current.name.trim().toLowerCase()
  return variableGroupsForScope(groups, targetScope).find(group => group.id !== 'default' && group.name.trim().toLowerCase() === name)?.id ?? 'default'
}

export function normalizeVariableGroups(rawGroups: unknown, sourceVariables: unknown[], createId = () => crypto.randomUUID()): NormalizedVariableGroups {
  type Candidate = GraphVariableGroup & { sourceId: string; explicitScope?: VariableScope }

  const defaultGroup: GraphVariableGroup = { id: 'default', name: 'Default' }
  const candidates: Candidate[] = []
  const groupIds = new Set<string>(['default'])
  let generatedSuffix = 1
  const uniqueGeneratedId = () => {
    const base = createId()
    if (!groupIds.has(base)) return base
    let id = `${base}-${generatedSuffix++}`
    while (groupIds.has(id)) id = `${base}-${generatedSuffix++}`
    return id
  }
  const addCandidate = (id: string, name: string, collapsed = false, scope?: VariableScope) => {
    const cleanId = id.trim()
    const cleanName = name.trim()
    if (!cleanId || !cleanName || groupIds.has(cleanId)) return
    groupIds.add(cleanId)
    candidates.push({ id: cleanId, sourceId: cleanId, name: cleanName, collapsed, scope, explicitScope: scope })
  }

  for (const value of Array.isArray(rawGroups) ? rawGroups : []) {
    const group = value as Record<string, unknown> | null
    if (group?.id === 'default') {
      defaultGroup.collapsed = Boolean(group.collapsed)
      continue
    }
    const scope = group?.scope === 'instance' ? 'instance' : group?.scope === 'execution' ? 'execution' : undefined
    addCandidate(String(group?.id ?? ''), String(group?.name ?? ''), Boolean(group?.collapsed), scope)
  }

  for (const value of sourceVariables) {
    const variable = value as Record<string, unknown> | null
    const legacyName = String(variable?.group ?? '').trim()
    if (!legacyName || legacyName.toLowerCase() === 'default') continue
    const exists = candidates.some(group => group.explicitScope !== 'instance' && group.name.trim().toLowerCase() === legacyName.toLowerCase())
    if (!exists) addCandidate(uniqueGeneratedId(), legacyName, false, 'execution')
  }

  const usageBySourceId = new Map<string, Set<VariableScope>>()
  for (const value of sourceVariables) {
    const variable = value as Record<string, unknown> | null
    const groupId = String(variable?.groupId ?? '').trim()
    if (!groupId || groupId === 'default') continue
    const scopes = usageBySourceId.get(groupId) ?? new Set<VariableScope>()
    scopes.add(variable?.scope === 'instance' ? 'instance' : 'execution')
    usageBySourceId.set(groupId, scopes)
  }

  const groups: GraphVariableGroup[] = [defaultGroup]
  const sourceScopeIds = new Map<string, string>()
  for (const candidate of candidates) {
    if (candidate.explicitScope) {
      groups.push({ id: candidate.id, name: candidate.name, collapsed: candidate.collapsed, scope: candidate.explicitScope })
      sourceScopeIds.set(`${candidate.sourceId}\0${candidate.explicitScope}`, candidate.id)
      continue
    }
    const usage = usageBySourceId.get(candidate.sourceId) ?? new Set<VariableScope>()
    const primaryScope: VariableScope = usage.has('instance') && !usage.has('execution') ? 'instance' : 'execution'
    groups.push({ id: candidate.id, name: candidate.name, collapsed: candidate.collapsed, scope: primaryScope })
    sourceScopeIds.set(`${candidate.sourceId}\0${primaryScope}`, candidate.id)
    if (usage.has('execution') && usage.has('instance')) {
      const splitId = uniqueGeneratedId()
      groupIds.add(splitId)
      groups.push({ id: splitId, name: candidate.name, collapsed: candidate.collapsed, scope: 'instance' })
      sourceScopeIds.set(`${candidate.sourceId}\0instance`, splitId)
      sourceScopeIds.set(`${candidate.sourceId}\0execution`, candidate.id)
    }
  }

  const groupNameScopeIds = new Map<string, string>()
  for (const group of groups) {
    if (group.id === 'default') continue
    const scope = variableGroupScope(group)!
    const key = `${group.name.trim().toLowerCase()}\0${scope}`
    if (!groupNameScopeIds.has(key)) groupNameScopeIds.set(key, group.id)
  }

  return {
    groups,
    resolveGroupId(requestedGroupId: string, legacyGroupName: string, scope: VariableScope) {
      const requested = requestedGroupId.trim()
      if (requested && requested !== 'default') {
        const mapped = sourceScopeIds.get(`${requested}\0${scope}`)
        if (mapped) return mapped
      }
      const legacyName = legacyGroupName.trim().toLowerCase()
      if (legacyName && legacyName !== 'default') {
        const mapped = groupNameScopeIds.get(`${legacyName}\0${scope}`)
        if (mapped) return mapped
      }
      return 'default'
    }
  }
}
