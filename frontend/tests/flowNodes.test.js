import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

function assert(value, message) {
  if (!value) throw new Error(message)
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..')
const source = name => readFileSync(resolve(root, 'frontend/src', name), 'utf8')
const flowNodes = JSON.parse(readFileSync(resolve(root, 'nodes/SysFlowControl.json'), 'utf8'))

const equalInteger = flowNodes.find(node => node.name === 'EqualInteger')
assert(equalInteger, 'SysFlowControl.json must keep EqualInteger')
const equalString = flowNodes.find(node => node.name === 'EqualString')
assert(equalString, 'SysFlowControl.json must register EqualString')
assert(equalString.title === '等于(字符串)==', 'EqualString must be titled 等于(字符串)==')
assert(equalString.inputs.map(port => port.port_id).join(',') === '0,1,2', 'EqualString must keep exec/a/b port IDs aligned with EqualInteger')
assert(equalString.inputs.every(port => port.type !== 'data' || port.data_type === 'String'), 'EqualString comparisons must accept strings only')
assert(equalString.outputs.map(port => port.port_id).join(',') === '0,1', 'EqualString must expose false/true exec outputs')

const runtimeSchemas = source('editor/runtimeNodeSchemas.ts')
assert(runtimeSchemas.includes("EqualString: { typeId: 'origin.flow.equal-string', inputs: ['exec', 'a', 'b'], outputs: ['false', 'true'] }"), 'legacy schemas must map the EqualString class')

const graph = readFileSync(resolve(root, 'graph.go'), 'utf8')
assert(graph.includes(`"origin.flow.equal-string": {
		Inputs: map[string]string{"exec": "exec", "a": "string", "b": "string"}, Outputs: map[string]string{"false": "exec", "true": "exec"},
	}`), 'graph.go must publish the equal-string port table')

const execution = readFileSync(resolve(root, 'execution.go'), 'utf8')
assert(execution.includes('case "origin.flow.equal-string":'), 'desktop executor must run equal-string')

const legacy = readFileSync(resolve(root, 'legacy.go'), 'utf8')
assert(legacy.includes(`"EqualString":                {"origin.flow.equal-string", []string{"exec", "a", "b"}, []string{"false", "true"}},`), 'legacy mapping must import/export the EqualString class')

console.log('flowNodes tests passed')
