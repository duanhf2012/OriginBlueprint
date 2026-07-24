import type { ValidationIssue } from './platform'

export interface SaveGateDecision {
  blocked: boolean
  blockingIssues: ValidationIssue[]
}

export function saveGateDecision(issues: ValidationIssue[], strict: boolean): SaveGateDecision {
  const blockingIssues = issues.filter(issue => {
    if (issue.target) return false
    if (issue.blocksSave) return true
    if (!strict || issue.severity !== 'error') return false
    return issue.code !== 'flow.possible-cycle'
  })
  return { blocked: blockingIssues.length > 0, blockingIssues }
}

