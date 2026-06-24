<script setup lang="ts">
import { computed, ref } from 'vue'
import { Ref } from 'rete-vue-plugin'
import type { BlueprintNode } from './types'
import { entryBindingBadgeLabel, entryBindingTitle } from './implicitEntryLinks'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: BlueprintNode; emit: (signal: unknown) => void }>()
const branchRevision = ref(0)
const inputs = computed(() => Object.entries(props.data.inputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const hiddenOutputKeys = computed(() => new Set(props.data.dynamicBranch?.hiddenOutputKeys ?? []))
const outputs = computed(() => Object.entries(props.data.outputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1]) && !hiddenOutputKeys.value.has(entry[0]) && !isOverflowBranchOutput(entry[0])))
const normalInputs = computed(() => props.data.dynamicBranch ? inputs.value.filter(([key]) => key !== props.data.dynamicBranch?.controlInput) : inputs.value)
const normalOutputs = computed(() => props.data.dynamicBranch ? outputs.value.filter(([key]) => key === props.data.dynamicBranch?.defaultOutput) : outputs.value)
const branchValues = computed(() => {
  branchRevision.value
  const key = props.data.dynamicBranch?.controlInput
  const control = key ? props.data.inputs[key]?.control as { value?: unknown[] } | undefined : undefined
  const value = control?.value
  return Array.isArray(value) ? value : []
})
const branchRows = computed(() => {
  const config = props.data.dynamicBranch
  if (!config) return []
  return branchValues.value.slice(0, config.maxBranches).map((value, index) => ({
    value,
    outputKey: `${config.outputPrefix}${config.outputStartIndex + index}`,
    index
  }))
})
const branchOutputRows = computed(() => {
  const config = props.data.dynamicBranch
  if (!config) return []
  const count = Math.min(branchValues.value.length, config.maxBranches)
  return Array.from({ length: count }, (_, index) => {
    const outputIndex = config.outputStartIndex + index
    return {
      outputKey: `${config.outputPrefix}${outputIndex}`,
      index
    }
  })
})
const rows = computed(() => Math.max(inputs.value.length, outputs.value.length))
const normalRows = computed(() => Math.max(normalInputs.value.length, normalOutputs.value.length))
const hasEntryBinding = computed(() => Object.values(props.data.portStates?.inputs ?? {}).some(state => Boolean(state?.entryBinding)))
const PORT_COLUMN_GAP = 6
const NODE_HORIZONTAL_PADDING = 20
const SOCKET_GUTTER_WIDTH = 36
const DEFAULT_CONTROL_WIDTH = 62
const DEFAULT_LABEL_MIN_WIDTH = 28
const ARRAY_CONTROL_WIDTH = 124
const FILE_CONTROL_WIDTH = 157
const ENTRY_BADGE_MIN_WIDTH = 56
const BRANCH_ACTION_WIDTH = 96

function estimateTextWidth(value: string | undefined, min = 0, max = 220) {
  let width = 0
  for (const char of String(value ?? '')) width += /[\u4e00-\u9fff\uff00-\uffef]/.test(char) ? 12 : 7
  return Math.max(min, Math.min(max, Math.ceil(width)))
}

function estimateControlWidth(control: unknown) {
  const value = (control as { value?: unknown; mode?: string } | undefined)?.value
  const mode = (control as { mode?: string } | undefined)?.mode
  if (mode === 'open' || mode === 'save') return FILE_CONTROL_WIDTH
  if (Array.isArray(value)) return ARRAY_CONTROL_WIDTH
  if (typeof value === 'boolean') return 64
  return DEFAULT_CONTROL_WIDTH
}

function inputContentWidth(entry: [string, NonNullable<typeof inputs.value[number][1]>]) {
  const [key, port] = entry
  const labelWidth = estimateTextWidth(port.label, DEFAULT_LABEL_MIN_WIDTH, hasEntryBinding.value ? 170 : 220)
  const controlWidth = port.control && port.showControl ? estimateControlWidth(port.control) + 10 : 0
  const bindingLabel = inputEntryBindingLabel(key)
  const bindingWidth = bindingLabel ? Math.max(ENTRY_BADGE_MIN_WIDTH, estimateTextWidth(bindingLabel, ENTRY_BADGE_MIN_WIDTH, 260)) + 8 : 0
  return SOCKET_GUTTER_WIDTH + labelWidth + controlWidth + bindingWidth
}

function outputContentWidth(entry: [string, NonNullable<typeof outputs.value[number][1]>]) {
  return SOCKET_GUTTER_WIDTH + estimateTextWidth(entry[1].label, DEFAULT_LABEL_MIN_WIDTH, 220)
}

function maxContentWidth<T>(items: T[], measure: (item: T) => number) {
  return items.reduce((width, item) => Math.max(width, measure(item)), 0)
}

const inputColumnWidth = computed(() => {
  const measured = maxContentWidth(inputs.value, inputContentWidth)
  return props.data.dynamicBranch ? Math.max(measured, SOCKET_GUTTER_WIDTH + 48 + 58 + 18 + BRANCH_ACTION_WIDTH) : measured
})
const outputColumnWidth = computed(() => maxContentWidth(outputs.value, outputContentWidth))
const nodeWidth = computed(() => Math.max(
  props.data.width ?? 230,
  inputColumnWidth.value + outputColumnWidth.value + PORT_COLUMN_GAP + NODE_HORIZONTAL_PADDING
))

function portFilled(side: 'inputs' | 'outputs', key: string) {
  return props.data.portStates?.[side][key]?.filled ?? false
}

function socketPayload(side: 'inputs' | 'outputs', key: string, socket: { name: string }) {
  return { name: socket.name, filled: portFilled(side, key) }
}

function portClass(side: 'inputs' | 'outputs', key: string, socket: { name: string }) {
  return [socketClassName(socket.name), { filled: portFilled(side, key) }]
}

function inputEntryBindingLabel(key: string) {
  return entryBindingBadgeLabel(props.data.portStates?.inputs[key]?.entryBinding)
}

function inputEntryBindingTitle(key: string) {
  return entryBindingTitle(props.data.portStates?.inputs[key]?.entryBinding)
}

function openEntryBindingMenu(event: MouseEvent, key: string, socket: { name: string }) {
  if (socket.name === 'exec') return
  if ((event.target as HTMLElement).closest('input, textarea, select, button')) return
  ;(event.currentTarget as HTMLElement).dispatchEvent(new CustomEvent('origin-entry-binding-menu', {
    bubbles: true,
    detail: { nodeId: props.data.id, inputKey: key, clientX: event.clientX, clientY: event.clientY }
  }))
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
  branchRevision.value++
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
  <article class="blueprint-node" :class="[`kind-${data.kind ?? 'function'}`, { selected: data.selected, compact: data.compact, legacy: Boolean(data.legacyClass), 'has-entry-binding': hasEntryBinding, 'reference-highlighted': data.referenceHighlighted }]" :style="{ width: `${nodeWidth}px` }">
    <header class="blueprint-title">
      <span class="node-icon">&#9670;</span>
      <span class="title-text">{{ data.label }}</span>
      <span v-if="data.legacyClass" class="legacy-badge">COMPAT</span>
      <span v-if="data.dynamicOutputs" class="dynamic-actions"><button @pointerdown.stop.prevent="changeOutputs(-1, $event)">-</button><button @pointerdown.stop.prevent="changeOutputs(1, $event)">+</button></span>
    </header>

    <div v-if="data.dynamicBranch" class="ports">
      <div v-for="index in normalRows" :key="`normal-${index}`" class="port-row">
        <div v-if="normalInputs[index - 1]" class="port input-port" :class="portClass('inputs', normalInputs[index - 1][0], normalInputs[index - 1][1].socket)" :style="socketStyle(normalInputs[index - 1][1].socket.name)" @contextmenu.stop.prevent="openEntryBindingMenu($event, normalInputs[index - 1][0], normalInputs[index - 1][1].socket)">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: normalInputs[index - 1][0], nodeId: data.id, payload: socketPayload('inputs', normalInputs[index - 1][0], normalInputs[index - 1][1].socket) }" />
          <span class="port-label">{{ normalInputs[index - 1][1].label }}</span>
          <span v-if="inputEntryBindingLabel(normalInputs[index - 1][0])" class="entry-binding-badge" :title="inputEntryBindingTitle(normalInputs[index - 1][0])">{{ inputEntryBindingLabel(normalInputs[index - 1][0]) }}</span>
          <Ref v-if="normalInputs[index - 1][1].control && normalInputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: normalInputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div class="port-spacer middle-spacer"></div>

        <div v-if="normalOutputs[index - 1]" class="port output-port" :class="portClass('outputs', normalOutputs[index - 1][0], normalOutputs[index - 1][1].socket)" :style="socketStyle(normalOutputs[index - 1][1].socket.name)">
          <span class="port-label">{{ normalOutputs[index - 1][1].label }}</span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: normalOutputs[index - 1][0], nodeId: data.id, payload: socketPayload('outputs', normalOutputs[index - 1][0], normalOutputs[index - 1][1].socket) }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
      <div v-for="row in branchRows" :key="`branch-${row.index}`" class="port-row branch-row" @pointerdown.stop @dblclick.stop.prevent>
        <div class="port input-port branch-input" :class="data.inputs[data.dynamicBranch.controlInput] ? portClass('inputs', data.dynamicBranch.controlInput, data.inputs[data.dynamicBranch.controlInput]!.socket) : []" :style="data.inputs[data.dynamicBranch.controlInput] ? socketStyle(data.inputs[data.dynamicBranch.controlInput]!.socket.name) : undefined" @contextmenu.stop.prevent="data.inputs[data.dynamicBranch.controlInput] && openEntryBindingMenu($event, data.dynamicBranch.controlInput, data.inputs[data.dynamicBranch.controlInput]!.socket)">
          <Ref v-if="row.index === 0 && data.inputs[data.dynamicBranch.controlInput]" class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: data.dynamicBranch.controlInput, nodeId: data.id, payload: socketPayload('inputs', data.dynamicBranch.controlInput, data.inputs[data.dynamicBranch.controlInput]!.socket) }" />
          <span v-else class="socket-ref branch-socket-spacer"></span>
          <span v-if="row.index === 0 && data.inputs[data.dynamicBranch.controlInput]" class="port-label">{{ data.inputs[data.dynamicBranch.controlInput]!.label }}</span>
          <span v-else class="port-label branch-label-spacer"></span>
          <input class="branch-value" :value="row.value" :type="dynamicControl()?.itemType === 'number' ? 'number' : 'text'" @pointerdown.stop @dblclick.stop @input="updateBranchValue(row.index, $event)" />
          <button class="branch-remove" title="Remove branch" @pointerdown.stop.prevent="removeBranch(row.index)">-</button>
        </div>
        <div class="port-spacer middle-spacer"></div>
        <div v-if="branchOutputRows[row.index] && data.outputs[branchOutputRows[row.index].outputKey]" class="port output-port branch-output" :class="portClass('outputs', branchOutputRows[row.index].outputKey, data.outputs[branchOutputRows[row.index].outputKey]!.socket)" :style="socketStyle(data.outputs[branchOutputRows[row.index].outputKey]!.socket.name)">
          <span class="port-label"></span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: branchOutputRows[row.index].outputKey, nodeId: data.id, payload: socketPayload('outputs', branchOutputRows[row.index].outputKey, data.outputs[branchOutputRows[row.index].outputKey]!.socket) }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
      <div class="port-row branch-actions-row" @pointerdown.stop @dblclick.stop.prevent>
        <div class="branch-actions-inline"><button :disabled="branchValues.length >= data.dynamicBranch.maxBranches" @pointerdown.stop.prevent="addBranch" @dblclick.stop.prevent>+ Item</button></div>
        <div class="port-spacer middle-spacer"></div>
        <div class="port-spacer"></div>
      </div>
    </div>

    <div v-else class="ports">
      <div v-for="index in rows" :key="index" class="port-row">
        <div v-if="inputs[index - 1]" class="port input-port" :class="portClass('inputs', inputs[index - 1][0], inputs[index - 1][1].socket)" :style="socketStyle(inputs[index - 1][1].socket.name)" @contextmenu.stop.prevent="openEntryBindingMenu($event, inputs[index - 1][0], inputs[index - 1][1].socket)">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: inputs[index - 1][0], nodeId: data.id, payload: socketPayload('inputs', inputs[index - 1][0], inputs[index - 1][1].socket) }" />
          <span class="port-label">{{ inputs[index - 1][1].label }}</span>
          <span v-if="inputEntryBindingLabel(inputs[index - 1][0])" class="entry-binding-badge" :title="inputEntryBindingTitle(inputs[index - 1][0])">{{ inputEntryBindingLabel(inputs[index - 1][0]) }}</span>
          <Ref v-if="inputs[index - 1][1].control && inputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: inputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div class="port-spacer middle-spacer"></div>

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
.blueprint-node { --accent: #4474bf; position: relative; overflow: visible; border: 1px solid color-mix(in srgb, var(--accent) 64%, transparent); border-radius: 4px; background: linear-gradient(145deg, #ffffff14, transparent 42%), linear-gradient(100deg, #191919ee, #101010eb); box-shadow: 0 5px 13px #0009, inset 0 1px #ffffff18; color: #ddd; cursor: default; user-select: none; }
.blueprint-node.kind-event { --accent: #bd202f; }
.blueprint-node.kind-function { --accent: #4f9a7f; }
.blueprint-node.kind-variable { --accent: #805aa8; }
.blueprint-node.legacy { --accent: #b77b32; border-style: dashed; }
.legacy-badge { margin-left: 4px; padding: 1px 4px; border: 1px solid #ffe0a455; border-radius: 2px; background: #2a1d0d99; color: #ffe0a4; font-size: 8px; letter-spacing: .5px; }
.blueprint-node.selected { outline: 2px solid #f5b642; outline-offset: 2px; box-shadow: 0 0 12px #f5b64255; }
.blueprint-node.reference-highlighted { outline: 3px solid #18d4ff; outline-offset: 3px; box-shadow: 0 0 0 1px #e7fbff88, 0 0 22px #18d4ffcc, 0 5px 13px #0009; }
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
.port-row { min-height: 27px; display: grid; grid-template-columns: max-content minmax(6px, 1fr) max-content; column-gap: 0; align-items: center; }
.blueprint-node.has-entry-binding .port-row { grid-template-columns: max-content minmax(6px, 1fr) max-content; column-gap: 0; }
.port { min-width: 0; display: flex; align-items: center; gap: 6px; font: 12px Consolas, monospace; }
.input-port { justify-content: flex-start; }
.output-port { justify-content: flex-end; text-align: right; }
.socket-ref { flex: 0 0 16px; display: flex; align-items: center; justify-content: center; }
.input-port .socket-ref { margin-left: 8px; }
.output-port .socket-ref { margin-right: 8px; }
.port-label { color: var(--socket-label-color); white-space: nowrap; }
.input-port .port-label { flex: 0 0 auto; min-width: max-content; }
.entry-binding-badge { flex: 0 1 auto; box-sizing: border-box; max-width: 180px; min-width: 0; overflow: hidden; padding: 1px 7px; border: 1px solid #33c5e8; border-radius: 2px; background: linear-gradient(90deg, #0b2e38e6, #101b20d9); color: #bff4ff; font: 10px "Segoe UI", sans-serif; text-overflow: ellipsis; white-space: nowrap; box-shadow: inset 0 0 0 1px #ffffff10, 0 0 5px #33c5e833; }
.port.socket-exec { min-height: 25px; }
.port:not(.filled) :deep(.blueprint-socket:not(.exec)) { background: #101010; box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff24; }
.port.filled :deep(.blueprint-socket:not(.exec)) { background: var(--socket-fill); box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff55, 0 0 4px var(--socket-color); }
.port:not(.filled) :deep(.blueprint-socket.exec path) { fill: #101010; stroke-width: 1.1; }
.port.filled :deep(.blueprint-socket.exec path) { fill: var(--socket-fill); stroke-width: .75; }
.control-ref { margin-left: auto; margin-right: 18px; display: flex; align-items: center; }
.port-spacer { min-height: 1px; }
.branch-row { min-height: 25px; }
.branch-input { position: relative; z-index: 2; width: calc(100% + 23px); display: grid; grid-template-columns: 24px 22px 1fr 18px; gap: 5px; justify-content: stretch; }
.branch-input .socket-ref { margin-left: 8px; }
.branch-socket-spacer { width: 16px; height: 1px; }
.branch-label-spacer { width: 22px; }
.branch-value { width: 58px; height: 18px; justify-self: end; padding: 1px 4px; border: 1px solid #555; border-radius: 1px; background: #e9e9e9; color: #111; font: 11px Consolas, monospace; }
.branch-remove { position: relative; z-index: 3; width: 18px; height: 18px; padding: 0; border: 1px solid #555; border-radius: 1px; background: #2b2b2b; color: #bbb; font: 12px Consolas, monospace; }
.branch-actions-row { min-height: 22px; }
.branch-actions-inline { display: flex; justify-content: flex-end; padding-right: 4px; }
.branch-actions-inline button { width: 72px; height: 20px; border: 1px solid #555; border-radius: 2px; background: #252525; color: #bbb; font: 11px Consolas, monospace; }
.branch-actions-inline button:disabled { opacity: .45; }
</style>
