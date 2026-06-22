<script setup lang="ts">
import { computed } from 'vue'
import type { BlueprintConnection } from './types'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: BlueprintConnection; path: string }>()
const socketClass = computed(() => socketClassName(props.data.socketType))
const styleVariables = computed(() => socketStyle(props.data.socketType))
const pathBounds = computed(() => {
  const values = props.path.match(/-?\d+(?:\.\d+)?(?:e[-+]?\d+)?/gi)?.map(Number).filter(Number.isFinite) ?? []
  if (values.length < 2) return { x: 0, y: 0, width: 1, height: 1 }
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (let index = 0; index < values.length - 1; index += 2) {
    const x = values[index]
    const y = values[index + 1]
    minX = Math.min(minX, x)
    minY = Math.min(minY, y)
    maxX = Math.max(maxX, x)
    maxY = Math.max(maxY, y)
  }
  const padding = 24
  return {
    x: minX - padding,
    y: minY - padding,
    width: Math.max(maxX - minX + padding * 2, 1),
    height: Math.max(maxY - minY + padding * 2, 1)
  }
})
const connectionStyle = computed(() => ({
  ...styleVariables.value,
  left: `${pathBounds.value.x}px`,
  top: `${pathBounds.value.y}px`,
  width: `${pathBounds.value.width}px`,
  height: `${pathBounds.value.height}px`
}))
const viewBox = computed(() => `${pathBounds.value.x} ${pathBounds.value.y} ${pathBounds.value.width} ${pathBounds.value.height}`)

function select(event: MouseEvent) {
  ;(event.currentTarget as SVGElement).dispatchEvent(new CustomEvent('origin-connection-select', {
    bubbles: true,
    detail: { id: props.data.id, additive: event.ctrlKey || event.metaKey }
  }))
}

function remove(event: MouseEvent) {
  event.preventDefault()
  ;(event.currentTarget as SVGElement).dispatchEvent(new CustomEvent('origin-connection-delete', {
    bubbles: true,
    detail: { id: props.data.id }
  }))
}
</script>

<template>
  <svg v-if="!data.hidden" class="blueprint-connection" :class="[socketClass, { selected: data.selected }]" :style="connectionStyle" :viewBox="viewBox" data-testid="connection" :data-connection-id="data.id" @click.stop="select" @contextmenu.stop="remove">
    <path class="connection-hit-area" :d="path" />
    <path class="connection-line" :d="path" />
  </svg>
</template>

<style scoped>
.blueprint-connection { --connection-color: #00a8e8; position: absolute; overflow: visible; pointer-events: none; }
.blueprint-connection path { fill: none; pointer-events: auto; }
.connection-hit-area { stroke: transparent !important; stroke-width: 13px !important; vector-effect: non-scaling-stroke; }
.connection-line { stroke: var(--connection-color); stroke-width: 1.55px; filter: drop-shadow(0 1px 1px #000); pointer-events: none !important; vector-effect: non-scaling-stroke; }
.blueprint-connection.socket-exec .connection-line { stroke-width: 2.5px; }
.blueprint-connection:hover .connection-line { stroke: #fff; stroke-width: 4px; }
.blueprint-connection.selected .connection-line { stroke: #f5b642; stroke-width: 5px; filter: drop-shadow(0 0 4px #f5b642aa); }
</style>
