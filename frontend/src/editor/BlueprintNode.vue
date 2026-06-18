<script setup lang="ts">
import { computed } from 'vue'
import { Ref } from 'rete-vue-plugin'
import type { BlueprintNode } from './types'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: BlueprintNode; emit: (signal: unknown) => void }>()
const inputs = computed(() => Object.entries(props.data.inputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const outputs = computed(() => Object.entries(props.data.outputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const rows = computed(() => Math.max(inputs.value.length, outputs.value.length))

function changeOutputs(delta: number, event: MouseEvent) {
  ;(event.currentTarget as HTMLElement).dispatchEvent(new CustomEvent('origin-dynamic-output', { bubbles: true, detail: { nodeId: props.data.id, delta } }))
}
</script>

<template>
  <article class="blueprint-node" :class="[`kind-${data.kind ?? 'function'}`, `execution-${data.executionState ?? 'idle'}`, { selected: data.selected, compact: data.compact, legacy: Boolean(data.legacyClass) }]" :style="{ width: `${data.width ?? 230}px` }">
    <header class="blueprint-title">
      <span class="node-icon">◇</span>
      <span>{{ data.label }}</span>
      <small v-if="data.subtitle">{{ data.subtitle }}</small>
      <span v-if="data.legacyClass" class="legacy-badge">COMPAT</span>
      <span v-if="data.dynamicOutputs" class="dynamic-actions"><button @pointerdown.stop @click="changeOutputs(-1, $event)">−</button><button @pointerdown.stop @click="changeOutputs(1, $event)">＋</button></span>
    </header>

    <div class="ports">
      <div v-for="index in rows" :key="index" class="port-row">
        <div v-if="inputs[index - 1]" class="port input-port" :class="socketClassName(inputs[index - 1][1].socket.name)" :style="socketStyle(inputs[index - 1][1].socket.name)">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: inputs[index - 1][0], nodeId: data.id, payload: inputs[index - 1][1].socket }" />
          <span class="port-label">{{ inputs[index - 1][1].label }}</span>
          <Ref v-if="inputs[index - 1][1].control && inputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: inputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div v-if="outputs[index - 1]" class="port output-port" :class="socketClassName(outputs[index - 1][1].socket.name)" :style="socketStyle(outputs[index - 1][1].socket.name)">
          <span class="port-label">{{ outputs[index - 1][1].label }}</span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: outputs[index - 1][0], nodeId: data.id, payload: outputs[index - 1][1].socket }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.blueprint-node { --accent: #4474bf; position: relative; overflow: visible; border: 1px solid color-mix(in srgb, var(--accent) 64%, transparent); border-radius: 4px; background: linear-gradient(100deg, #18181842, #10101038); box-shadow: 0 5px 13px #0009, inset 0 1px #ffffff10; color: #ddd; cursor: default; user-select: none; backdrop-filter: blur(2px); }
.blueprint-node.kind-event { --accent: #bd202f; }.blueprint-node.kind-function { --accent: #4f9a7f; }.blueprint-node.kind-variable { --accent: #805aa8; }
.blueprint-node.legacy { --accent: #b77b32; border-style: dashed; }.legacy-badge { margin-left: 4px; padding: 1px 4px; border: 1px solid #ffe0a455; border-radius: 2px; background: #2a1d0d99; color: #ffe0a4; font-size: 8px; letter-spacing: .5px; }
.blueprint-node.selected { outline: 2px solid #f5b642; outline-offset: 2px; box-shadow: 0 0 12px #f5b64255; }
.blueprint-node.execution-running { outline: 3px solid #f1c232; outline-offset: 3px; box-shadow: 0 0 18px #f1c23299; }
.blueprint-node.execution-completed { box-shadow: 0 0 13px #3ecf6f88; }
.blueprint-node.execution-completed::after, .blueprint-node.execution-error::after { content: ""; position: absolute; inset: -3px; border: 2px solid #3ecf6f; border-radius: 6px; pointer-events: none; }
.blueprint-node.execution-error { box-shadow: 0 0 18px #f0444499; }.blueprint-node.execution-error::after { border-color: #f04444; }
.blueprint-node.compact { box-shadow: 0 2px 6px #0008; }
.blueprint-node.compact .blueprint-title { height: 25px; font-size: 12px; }
.blueprint-node.compact .blueprint-title small { display: none; }
.blueprint-node.compact .ports { padding: 3px 0 4px; }
.blueprint-node.compact .port-row { min-height: 21px; }
.blueprint-title { height: 31px; display: flex; align-items: center; gap: 6px; padding: 0 8px; border-radius: 3px 3px 0 0; background: linear-gradient(90deg, var(--accent), color-mix(in srgb, var(--accent) 75%, #222)); color: white; font: 16px Arial, sans-serif; text-shadow: 0 1px #0008; cursor: move; }
.blueprint-title small { margin-left: auto; color: #ffffff99; font-size: 9px; text-transform: uppercase; }.node-icon { opacity: .8; }
.dynamic-actions { display: flex; gap: 2px; margin-left: 3px; }.dynamic-actions button { width: 19px; height: 18px; padding: 0; border: 1px solid #ffffff55; border-radius: 2px; background: #0003; color: white; line-height: 14px; }
.ports { padding: 7px 0 8px; }.port-row { min-height: 27px; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: center; }
.port { min-width: 0; display: flex; align-items: center; gap: 6px; font: 12px Consolas, monospace; }
.input-port { justify-content: flex-start; }.output-port { justify-content: flex-end; text-align: right; }.socket-ref { flex: 0 0 16px; display: flex; align-items: center; justify-content: center; }
.input-port .socket-ref { margin-left: 8px; }.output-port .socket-ref { margin-right: 8px; }.port-label { color: var(--socket-label-color); white-space: nowrap; }
.port.socket-exec { min-height: 25px; }
.control-ref { margin-left: auto; margin-right: 4px; display: flex; align-items: center; }.port-spacer { min-height: 1px; }
</style>
