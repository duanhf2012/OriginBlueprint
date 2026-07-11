import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const source = name => readFileSync(resolve(root, 'src', name), 'utf8')
const documentSource = source('editor/document.ts')
const typesSource = source('editor/types.ts')
const editorSource = source('editor/createEditor.ts')
const appSource = source('App.vue')
const zhCn = source('i18n/zh-CN.ts')
const enUs = source('i18n/en-US.ts')

assert(documentSource.includes('residualNodeDefaults?: Record<string, LegacyResidualNodeDefaults>'), 'legacy state must type residual defaults')
assert(documentSource.includes('hiddenEdgeOrdinals?: number[]'), 'legacy state must type hidden edge ordinals')
assert(documentSource.includes('legacyEdgeId?: string'), 'connection snapshots must retain legacy edge ids')
assert(documentSource.includes('legacyOrdinal?: number'), 'connection snapshots must retain legacy ordinals')
assert(typesSource.includes('legacyEdgeId?: string'), 'Rete connections must carry legacy edge ids')
assert(typesSource.includes('legacyOrdinal?: number'), 'Rete connections must carry legacy ordinals')
assert(editorSource.includes('legacyEdgeId: item.legacyEdgeId'), 'snapshot must serialize legacy edge ids')
assert(editorSource.includes('connection.legacyEdgeId = item.legacyEdgeId'), 'restore must copy legacy edge ids onto Rete connections')
assert(editorSource.includes('function historySnapshot'), 'undo history must include opaque legacy state')
assert(editorSource.includes('function restoreHistory'), 'undo must restore opaque legacy state')
assert(editorSource.includes('function pruneHiddenLegacyEdges'), 'node deletion must prune hidden legacy edges')
assert(editorSource.includes('hidden legacy connection(s)'), 'deletion status must report hidden legacy cleanup')
assert(appSource.includes('status.value = `${menuText.value.status.saveFailed}:'), 'save failures must show localized status plus backend detail')
assert(appSource.includes('} catch (error) {'), 'saveGraph must handle rejected export/save operations')
assert(zhCn.includes("saveFailed: '保存失败'"), 'Chinese locale must include save failure text')
assert(enUs.includes("saveFailed: 'Save failed'"), 'English locale must include save failure text')

console.log('legacyPersistence tests passed')
