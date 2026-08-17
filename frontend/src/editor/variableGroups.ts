import { variableScope, type GraphVariable, type GraphVariableGroup } from './document'

export interface VariableGroupUsage {
  localCount: number
  globalCount: number
  totalCount: number
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

export function variableGroupRemovalMessage(groupName: string, usage: VariableGroupUsage) {
  return `删除分组“${groupName}”？其中有局部变量 ${usage.localCount} 个、全局变量 ${usage.globalCount} 个；删除后它们都会移动到 Default。`
}

export function variableGroupNameExists(groups: GraphVariableGroup[], name: string, exceptId = '') {
  const key = name.trim().toLowerCase()
  return groups.some(group => group.id !== exceptId && group.name.trim().toLowerCase() === key)
}

export function moveVariablesToDefaultGroup(variables: GraphVariable[], groupId: string) {
  for (const variable of variables) if (variable.groupId === groupId) variable.groupId = 'default'
}

export function normalizeVariableGroups(rawGroups: unknown, sourceVariables: unknown[], createId = () => crypto.randomUUID()): GraphVariableGroup[] {
  const groups: GraphVariableGroup[] = [{ id: 'default', name: 'Default' }]
  const groupIds = new Set<string>(['default'])
  const groupNames = new Set<string>(['default'])
  const addGroup = (id: string, name: string, collapsed = false) => {
    const cleanId = id.trim()
    const cleanName = name.trim()
    if (!cleanId || !cleanName || groupIds.has(cleanId)) return
    groupIds.add(cleanId)
    groupNames.add(cleanName.toLowerCase())
    groups.push({ id: cleanId, name: cleanName, collapsed })
  }

  for (const value of Array.isArray(rawGroups) ? rawGroups : []) {
    const group = value as Record<string, unknown> | null
    if (group?.id === 'default') {
      groups[0].collapsed = Boolean(group.collapsed)
      continue
    }
    addGroup(String(group?.id ?? ''), String(group?.name ?? ''), Boolean(group?.collapsed))
  }
  for (const value of sourceVariables) {
    const variable = value as Record<string, unknown> | null
    const legacyName = String(variable?.group ?? '').trim()
    if (legacyName && legacyName.toLowerCase() !== 'default' && !groupNames.has(legacyName.toLowerCase())) {
      addGroup(createId(), legacyName)
    }
  }
  return groups
}
