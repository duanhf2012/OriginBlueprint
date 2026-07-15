import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const __dirname = dirname(fileURLToPath(import.meta.url))
const fixtureRoot = resolve(__dirname, '../../examples/verification-blueprints')
const fixture = relativePath => JSON.parse(readFileSync(resolve(fixtureRoot, relativePath), 'utf8'))

function assertFallbackPorts(relativePath, typeId, legacyClass, expectedInputs, expectedOutputs) {
  const document = fixture(relativePath)
  const nodes = document.nodes.filter(node => node.typeId === typeId)
  assert(nodes.length > 0, `${relativePath} must contain ${legacyClass}`)

  for (const node of nodes) {
    assert(node.properties?.legacyClass === legacyClass, `${relativePath}/${node.id} must provide an editor fallback class`)
    assert(node.properties?.legacyModule === 'verification.fixture', `${relativePath}/${node.id} must mark the fallback as test-only`)
    assert(JSON.stringify(node.properties?.legacyInputs?.map(port => port.key)) === JSON.stringify(expectedInputs), `${relativePath}/${node.id} fallback inputs must match the document connections`)
    assert(JSON.stringify(node.properties?.legacyOutputs?.map(port => port.key)) === JSON.stringify(expectedOutputs), `${relativePath}/${node.id} fallback outputs must match the document connections`)

    for (const connection of document.connections) {
      if (connection.source === node.id) assert(expectedOutputs.includes(connection.sourceOutput), `${relativePath}/${node.id} connection uses missing fallback output ${connection.sourceOutput}`)
      if (connection.target === node.id) assert(expectedInputs.includes(connection.targetInput), `${relativePath}/${node.id} connection uses missing fallback input ${connection.targetInput}`)
    }
  }
}

for (const relativePath of ['06_async_delay_resume.obp', 'functions/14_async_delay_function.obpf']) {
  assertFallbackPorts(
    relativePath,
    'origin.example.mock-delay-async',
    'MockDelayAsync',
    ['exec', 'delayMs', 'value', 'tag'],
    ['completed', 'value', 'tag'],
  )
}

assertFallbackPorts(
  '07_async_rpc_resume_to.obp',
  'origin.example.mock-rpc-async',
  'MockRpcAsync',
  ['exec', 'delayMs', 'succeed', 'successValue', 'failureCode', 'failureMessage'],
  ['succeeded', 'failed', 'value', 'errorCode', 'errorMessage'],
)

console.log('verificationFixtures tests passed')
