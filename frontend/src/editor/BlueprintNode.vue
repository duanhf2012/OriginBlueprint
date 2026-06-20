<script setup lang="ts">
import { computed } from 'vue'
import { Ref } from 'rete-vue-plugin'
import type { BlueprintNode } from './types'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: BlueprintNode; emit: (signal: unknown) => void }>()
const inputs = computed(() => Object.entries(props.data.inputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const hiddenOutputKeys = computed(() => new Set(props.data.dynamicBranch?.hiddenOutputKeys ?? []))
const outputs = computed(() => Object.entries(props.data.outputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1]) && !hiddenOutputKeys.value.has(entry[0]) && !isOverflowBranchOutput(entry[0])))
const normalInputs = computed(() => props.data.dynamicBranch ? inputs.value.filter(([key]) => key !== props.data.dynamicBranch?.controlInput) : inputs.value)
const normalOutputs = computed(() => props.data.dynamicBranch ? outputs.value.filter(([key]) => key === props.data.dynamicBranch?.defaultOutput) : outputs.value)
const branchValues = computed(() => {
  const key = props.data.dynamicBranch?.controlInput
  const control = key ? props.data.inputs[key]?.control as { value?: unknown[] } | undefined : undefined
  const value = control?.value
  return Array.isArray(value) ? value : []
})
const branchRows = computed(() => {
  const config = props.data.dynamicBranch
  if (!config) return []
  const indexes = new Set<number>()
  branchValues.value.slice(0, config.maxBranches).forEach((_, index) => indexes.add(config.outputStartIndex + index))
  for (const [key, state] of Object.entries(props.data.portStates?.outputs ?? {})) {
    if (!state.connected || !key.startsWith(config.outputPrefix) || config.hiddenOutputKeys?.includes(key)) continue
    const index = Number(key.slice(config.outputPrefix.length))
    if (Number.isFinite(index) && index >= config.outputStartIndex && index < config.outputStartIndex + config.maxBranches) indexes.add(index)
  }
  return [...indexes].sort((a, b) => a - b).map(outputIndex => ({
    value: branchValues.value[outputIndex - config.outputStartIndex] ?? '',
    outputKey: `${config.outputPrefix}${outputIndex}`,
    index: outputIndex - config.outputStartIndex
  }))
})
const rows = computed(() => Math.max(inputs.value.length, outputs.value.length))
const normalRows = computed(() => Math.max(normalInputs.value.length, normalOutputs.value.length))

function portFilled(side: 'inputs' | 'outputs', key: string) {
  return props.data.portStates?.[side][key]?.filled ?? false
}

function socketPayload(side: 'inputs' | 'outputs', key: string, socket: { name: string }) {
  return { name: socket.name, filled: portFilled(side, key) }
}

function portClass(side: 'inputs' | 'outputs', key: string, socket: { name: string }) {
  return [socketClassName(socket.name), { filled: portFilled(side, key) }]
}

function changeOutputs(delta: number, event: MouseEvent) {
  ;(event.currentTarget as HTMLElement).dispatchEvent(new CustomEvent('origin-dynamic-output', { bubbles: true, detail: { nodeId: props.data.id, delta } }))
}

function isOverflowBranchOutput(key: string) {
  const config = props.data.dynamicBranch
  if (!config || !key.startsWith(config.outputPrefix)) return false
  const index = Number(key.slice(config.outputPrefix.length))
  if (!Number.isFinite(index)) return false
  return index >= config.outputStartIndex + branchValues.value.length
}

function dynamicControl() {
  const key = props.data.dynamicBranch?.controlInput
  return key ? props.data.inputs[key]?.control as { value?: Array<string | number>; itemType?: 'string' | 'number'; setValue?: (value: unknown) => void } | undefined : undefined
}

function setBranchValues(values: Array<string | number>, countChanged: boolean) {
  const control = dynamicControl()
  control?.setValue?.(values)
  document.dispatchEvent(new CustomEvent('origin-control-change'))
  document.dispatchEvent(new CustomEvent('origin-dynamic-branch-change', { detail: { nodeId: props.data.id, count: values.length, countChanged } }))
}

function updateBranchValue(index: number, event: Event) {
  const control = dynamicControl()
  const values = [...branchValues.value] as Array<string | number>
  const raw = (event.target as HTMLInputElement).value
  values[index] = control?.itemType === 'number' ? Number(raw) : raw
  setBranchValues(values, false)
}

function addBranch() {
  const config = props.data.dynamicBranch
  if (!config || branchValues.value.length >= config.maxBranches) return
  const control = dynamicControl()
  setBranchValues([...(branchValues.value as Array<string | number>), control?.itemType === 'number' ? 0 : ''], true)
}

function removeBranch(index: number) {
  const values = [...branchValues.value] as Array<string | number>
  values.splice(index, 1)
  setBranchValues(values, true)
}
</script>

<template>
  <article class="blueprint-node" :class="[`kind-${data.kind ?? 'function'}`, `execution-${data.executionState ?? 'idle'}`, { selected: data.selected, compact: data.compact, legacy: Boolean(data.legacyClass) }]" :style="{ width: `${data.width ?? 230}px` }">
    <header class="blueprint-title">
      <span class="node-icon">&#9670;</span>
      <span class="title-text">{{ data.label }}</span>
      <span v-if="data.legacyClass" class="legacy-badge">COMPAT</span>
      <span v-if="data.dynamicOutputs" class="dynamic-actions"><button @pointerdown.stop @click="changeOutputs(-1, $event)">-</button><button @pointerdown.stop @click="changeOutputs(1, $event)">+</button></span>
    </header>

    <div v-if="data.dynamicBranch" class="ports">
      <div v-for="index in normalRows" :key="`normal-${index}`" class="port-row">
        <div v-if="normalInputs[index - 1]" class="port input-port" :class="portClass('inputs', normalInputs[index - 1][0], normalInputs[index - 1][1].socket)" :style="socketStyle(normalInputs[index - 1][1].socket.name)">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: normalInputs[index - 1][0], nodeId: data.id, payload: socketPayload('inputs', normalInputs[index - 1][0], normalInputs[index - 1][1].socket) }" />
          <span class="port-label">{{ normalInputs[index - 1][1].label }}</span>
          <Ref v-if="normalInputs[index - 1][1].control && normalInputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: normalInputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div v-if="normalOutputs[index - 1]" class="port output-port" :class="portClass('outputs', normalOutputs[index - 1][0], normalOutputs[index - 1][1].socket)" :style="socketStyle(normalOutputs[index - 1][1].socket.name)">
          <span class="port-label">{{ normalOutputs[index - 1][1].label }}</span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: normalOutputs[index - 1][0], nodeId: data.id, payload: socketPayload('outputs', normalOutputs[index - 1][0], normalOutputs[index - 1][1].socket) }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
      <div v-for="row in branchRows" :key="`branch-${row.index}`" class="port-row branch-row">
        <div class="port input-port branch-input" :class="data.inputs[data.dynamicBranch.controlInput] ? portClass('inputs', data.dynamicBranch.controlInput, data.inputs[data.dynamicBranch.controlInput]!.socket) : []" :style="data.inputs[data.dynamicBranch.controlInput] ? socketStyle(data.inputs[data.dynamicBranch.controlInput]!.socket.name) : undefined">
          <Ref v-if="row.index === 0 && data.inputs[data.dynamicBranch.controlInput]" class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: data.dynamicBranch.controlInput, nodeId: data.id, payload: socketPayload('inputs', data.dynamicBranch.controlInput, data.inputs[data.dynamicBranch.controlInput]!.socket) }" />
          <span v-else class="socket-ref branch-socket-spacer"></span>
          <span v-if="row.index === 0 && data.inputs[data.dynamicBranch.controlInput]" class="port-label">{{ data.inputs[data.dynamicBranch.controlInput]!.label }}</span>
          <span v-else class="port-label branch-label-spacer"></span>
          <input class="branch-value" :value="row.value" :type="dynamicControl()?.itemType === 'number' ? 'number' : 'text'" @pointerdown.stop @dblclick.stop @input="updateBranchValue(row.index, $event)" />
          <button class="branch-remove" title="Remove branch" @pointerdown.stop @click="removeBranch(row.index)">-</button>
        </div>
        <div v-if="data.outputs[row.outputKey]" class="port output-port branch-output" :class="portClass('outputs', row.outputKey, data.outputs[row.outputKey]!.socket)" :style="socketStyle(data.outputs[row.outputKey]!.socket.name)">
          <span class="port-label"></span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: row.outputKey, nodeId: data.id, payload: socketPayload('outputs', row.outputKey, data.outputs[row.outputKey]!.socket) }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
      <div class="port-row branch-actions-row">
        <div class="branch-actions-inline"><button :disabled="branchValues.length >= data.dynamicBranch.maxBranches" @pointerdown.stop @click="addBranch">+ Item</button></div>
        <div class="port-spacer"></div>
      </div>
    </div>

    <div v-else class="ports">
      <div v-for="index in rows" :key="index" class="port-row">
        <div v-if="inputs[index - 1]" class="port input-port" :class="portClass('inputs', inputs[index - 1][0], inputs[index - 1][1].socket)" :style="socketStyle(inputs[index - 1][1].socket.name)">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: inputs[index - 1][0], nodeId: data.id, payload: socketPayload('inputs', inputs[index - 1][0], inputs[index - 1][1].socket) }" />
          <span class="port-label">{{ inputs[index - 1][1].label }}</span>
          <Ref v-if="inputs[index - 1][1].control && inputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: inputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div v-if="outputs[index - 1]" class="port output-port" :class="portClass('outputs', outputs[index - 1][0], outputs[index - 1][1].socket)" :style="socketStyle(outputs[index - 1][1].socket.name)">
          <span class="port-label">{{ outputs[index - 1][1].label }}</span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: outputs[index - 1][0], nodeId: data.id, payload: socketPayload('outputs', outputs[index - 1][0], outputs[index - 1][1].socket) }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.blueprint-node { --accent: #4474bf; position: relative; overflow: visible; border: 1px solid color-mix(in srgb, var(--accent) 64%, transparent); border-radius: 4px; background: linear-gradient(100deg, #18181842, #10101038); box-shadow: 0 5px 13px #0009, inset 0 1px #ffffff10; color: #ddd; cursor: default; user-select: none; backdrop-filter: blur(2px); }
.blueprint-node.kind-event { --accent: #bd202f; }
.blueprint-node.kind-function { --accent: #4f9a7f; }
.blueprint-node.kind-variable { --accent: #805aa8; }
.blueprint-node.legacy { --accent: #b77b32; border-style: dashed; }
.legacy-badge { margin-left: 4px; padding: 1px 4px; border: 1px solid #ffe0a455; border-radius: 2px; background: #2a1d0d99; color: #ffe0a4; font-size: 8px; letter-spacing: .5px; }
.blueprint-node.selected { outline: 2px solid #f5b642; outline-offset: 2px; box-shadow: 0 0 12px #f5b64255; }
.blueprint-node.execution-running { outline: 3px solid #f1c232; outline-offset: 3px; box-shadow: 0 0 18px #f1c23299; }
.blueprint-node.execution-completed { box-shadow: 0 0 13px #3ecf6f88; }
.blueprint-node.execution-completed::after, .blueprint-node.execution-error::after { content: ""; position: absolute; inset: -3px; border: 2px solid #3ecf6f; border-radius: 6px; pointer-events: none; }
.blueprint-node.execution-error { box-shadow: 0 0 18px #f0444499; }
.blueprint-node.execution-error::after { border-color: #f04444; }
.blueprint-node.compact { box-shadow: 0 2px 6px #0008; }
.blueprint-node.compact .blueprint-title { height: 25px; font-size: 12px; }
.blueprint-node.compact .ports { padding: 3px 0 4px; }
.blueprint-node.compact .port-row { min-height: 21px; }
.blueprint-title { height: 31px; display: flex; align-items: center; gap: 6px; padding: 0 8px; border-radius: 3px 3px 0 0; background: linear-gradient(90deg, var(--accent), color-mix(in srgb, var(--accent) 75%, #222)); color: white; font: 16px Arial, sans-serif; text-shadow: 0 1px #0008; cursor: move; white-space: nowrap; }
.title-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.node-icon { opacity: .8; }
.dynamic-actions { display: flex; gap: 2px; margin-left: 3px; }
.dynamic-actions button { width: 19px; height: 18px; padding: 0; border: 1px solid #ffffff55; border-radius: 2px; background: #0003; color: white; line-height: 14px; }
.ports { padding: 7px 0 8px; }
.port-row { min-height: 27px; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: center; }
.port { min-width: 0; display: flex; align-items: center; gap: 6px; font: 12px Consolas, monospace; }
.input-port { justify-content: flex-start; }
.output-port { justify-content: flex-end; text-align: right; }
.socket-ref { flex: 0 0 16px; display: flex; align-items: center; justify-content: center; }
.input-port .socket-ref { margin-left: 8px; }
.output-port .socket-ref { margin-right: 8px; }
.port-label { color: var(--socket-label-color); white-space: nowrap; }
.port.socket-exec { min-height: 25px; }
.port:not(.filled) :deep(.blueprint-socket:not(.exec)) { background: #101010; box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff24; }
.port.filled :deep(.blueprint-socket:not(.exec)) { background: var(--socket-fill); box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff55, 0 0 4px var(--socket-color); }
.port:not(.filled) :deep(.blueprint-socket.exec path) { fill: #101010; stroke-width: 1.1; }
.port.filled :deep(.blueprint-socket.exec path) { fill: var(--socket-fill); stroke-width: .75; }
.control-ref { margin-left: auto; margin-right: 4px; display: flex; align-items: center; }
.port-spacer { min-height: 1px; }
.branch-row { min-height: 25px; }
.branch-input { gap: 5px; }
.branch-socket-spacer { width: 16px; height: 1px; margin-left: 8px; }
.branch-label-spacer { width: 1.5em; }
.branch-value { width: 54px; height: 18px; padding: 1px 4px; border: 1px solid #555; border-radius: 1px; background: #e9e9e9; color: #111; font: 11px Consolas, monospace; }
.branch-remove { width: 18px; height: 18px; padding: 0; border: 1px solid #555; border-radius: 1px; background: #2b2b2b; color: #bbb; font: 12px Consolas, monospace; }
.branch-actions-row { min-height: 22px; }
.branch-actions-inline { margin-left: 39px; }
.branch-actions-inline button { width: 72px; height: 20px; border: 1px solid #555; border-radius: 2px; background: #252525; color: #bbb; font: 11px Consolas, monospace; }
.branch-actions-inline button:disabled { opacity: .45; }
</style>
