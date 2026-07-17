import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname.replace(/^\/(.:)/, '$1')), '..', 'src')
const source = relative => fs.readFileSync(path.join(root, relative), 'utf8')

function assert(value, message) {
  if (!value) throw new Error(message)
}

const app = source('App.vue')
const platform = source('platform.ts')

assert(app.includes("from './saveGate'"), 'the app must use the shared save-gate policy')
assert(app.includes('async function validateForPersistence('), 'manual and automatic persistence need one validation gate')
assert((app.match(/await validateForPersistence\(/g) ?? []).length >= 2, 'manual save and autosave must both call the shared gate')
assert(app.includes('platform.saveRecoverySnapshot('), 'blocked persistence must write a recovery snapshot')
assert(app.includes('platform.deleteRecoverySnapshots('), 'successful persistence must clear recovery snapshots')
assert(platform.includes('SaveRecoverySnapshot') && platform.includes('ListRecoverySnapshots') && platform.includes('ReadRecoverySnapshot'), 'desktop recovery APIs must be exposed through the platform adapter')

console.log('coreGraphSaveGate tests passed')

