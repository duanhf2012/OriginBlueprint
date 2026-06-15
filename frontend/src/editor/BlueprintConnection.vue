<script setup lang="ts">
import type { BlueprintConnection } from './types'

const props = defineProps<{ data: BlueprintConnection; path: string }>()

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
  <svg class="blueprint-connection" :class="{ selected: data.selected }" data-testid="connection" :data-connection-id="data.id" @click.stop="select" @contextmenu.stop="remove">
    <path class="connection-hit-area" :d="path" />
    <path class="connection-line" :d="path" />
  </svg>
</template>

<style scoped>
.blueprint-connection { position: absolute; width: 9999px; height: 9999px; overflow: visible; pointer-events: none; }
.blueprint-connection path { fill: none; pointer-events: auto; }
.connection-hit-area { stroke: transparent !important; stroke-width: 13px !important; }
.connection-line { stroke: #d2d2d2; stroke-width: 3px; filter: drop-shadow(0 1px 1px #000); pointer-events: none !important; }
.blueprint-connection:hover .connection-line { stroke: #fff; stroke-width: 4px; }
.blueprint-connection.selected .connection-line { stroke: #f5b642; stroke-width: 5px; filter: drop-shadow(0 0 4px #f5b642aa); }
</style>
