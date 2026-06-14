<script setup lang="ts">
import { computed } from 'vue'
import { Ref } from 'rete-vue-plugin'
import type { BlueprintNode } from './types'

const props = defineProps<{ data: BlueprintNode; emit: (signal: unknown) => void }>()
const inputs = computed(() => Object.entries(props.data.inputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const outputs = computed(() => Object.entries(props.data.outputs).filter((entry): entry is [string, NonNullable<typeof entry[1]>] => Boolean(entry[1])))
const rows = computed(() => Math.max(inputs.value.length, outputs.value.length))
</script>

<template>
  <article class="blueprint-node" :class="[`kind-${data.kind ?? 'function'}`, { selected: data.selected, compact: data.compact }]" :style="{ width: `${data.width ?? 230}px` }">
    <header class="blueprint-title">
      <span class="node-icon">◇</span>
      <span>{{ data.label }}</span>
      <small v-if="data.subtitle">{{ data.subtitle }}</small>
    </header>

    <div class="ports">
      <div v-for="index in rows" :key="index" class="port-row">
        <div v-if="inputs[index - 1]" class="port input-port">
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'input', key: inputs[index - 1][0], nodeId: data.id, payload: inputs[index - 1][1].socket }" />
          <span class="port-label" :class="`label-${inputs[index - 1][1].socket.name}`">{{ inputs[index - 1][1].label }}</span>
          <Ref v-if="inputs[index - 1][1].control && inputs[index - 1][1].showControl" class="control-ref" :emit="emit" :data="{ type: 'control', payload: inputs[index - 1][1].control }" />
        </div>
        <div v-else class="port-spacer"></div>

        <div v-if="outputs[index - 1]" class="port output-port">
          <span class="port-label" :class="`label-${outputs[index - 1][1].socket.name}`">{{ outputs[index - 1][1].label }}</span>
          <Ref class="socket-ref" :emit="emit" :data="{ type: 'socket', side: 'output', key: outputs[index - 1][0], nodeId: data.id, payload: outputs[index - 1][1].socket }" />
        </div>
        <div v-else class="port-spacer"></div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.blueprint-node { --accent: #4474bf; position: relative; overflow: visible; border: 1px solid var(--accent); border-radius: 4px; background: linear-gradient(100deg, #181818, #101010); box-shadow: 0 5px 13px #0009; color: #ddd; cursor: default; user-select: none; }
.blueprint-node.kind-event { --accent: #bd202f; }.blueprint-node.kind-function { --accent: #4f9a7f; }.blueprint-node.kind-variable { --accent: #805aa8; }
.blueprint-node.selected { outline: 2px solid #f5b642; outline-offset: 2px; box-shadow: 0 0 12px #f5b64255; }
.blueprint-node.compact { box-shadow: 0 2px 6px #0008; }
.blueprint-node.compact .blueprint-title { height: 25px; font-size: 12px; }
.blueprint-node.compact .blueprint-title small { display: none; }
.blueprint-node.compact .ports { padding: 3px 0 4px; }
.blueprint-node.compact .port-row { min-height: 21px; }
.blueprint-title { height: 31px; display: flex; align-items: center; gap: 6px; padding: 0 8px; border-radius: 3px 3px 0 0; background: linear-gradient(90deg, var(--accent), color-mix(in srgb, var(--accent) 75%, #222)); color: white; font-size: 15px; text-shadow: 0 1px #0008; }
.blueprint-title small { margin-left: auto; color: #ffffff99; font-size: 9px; text-transform: uppercase; }.node-icon { opacity: .8; }
.ports { padding: 7px 0 8px; }.port-row { min-height: 27px; display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); align-items: center; }
.port { min-width: 0; display: flex; align-items: center; gap: 6px; font: 12px Consolas, monospace; }
.input-port { justify-content: flex-start; }.output-port { justify-content: flex-end; text-align: right; }.socket-ref { flex: 0 0 auto; display: flex; align-items: center; }
.input-port .socket-ref { margin-left: -8px; }.output-port .socket-ref { margin-right: -8px; }.port-label { white-space: nowrap; }
.label-boolean { color: #ff3945; }.label-string { color: #f032c0; }.label-integer, .label-number { color: #24c442; }.label-float { color: #5bd2b2; }.label-array { color: #e0b820; }.label-any { color: #55a7e9; }
.control-ref { margin-left: auto; margin-right: 4px; display: flex; align-items: center; }.port-spacer { min-height: 1px; }
</style>
