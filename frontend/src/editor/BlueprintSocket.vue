<script setup lang="ts">
import { computed } from 'vue'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: { name: string } }>()
const isExec = computed(() => props.data.name === 'exec')
const socketClass = computed(() => socketClassName(props.data.name))
const styleVariables = computed(() => socketStyle(props.data.name))
</script>

<template>
  <div class="blueprint-socket" :class="[socketClass, { exec: isExec }]" :style="styleVariables" :title="data.name">
    <svg v-if="isExec" viewBox="0 0 16 16" aria-hidden="true">
      <path d="M2.6 2.2H7.2L13.6 8L7.2 13.8H2.6Z" />
    </svg>
  </div>
</template>

<style scoped>
.blueprint-socket { width: 12px; height: 12px; border: 1px solid var(--socket-color); border-radius: 50%; background: var(--socket-fill); box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff55, 0 0 4px var(--socket-color); cursor: crosshair; transition: transform .1s, filter .1s; }
.blueprint-socket:hover { transform: scale(1.25); filter: brightness(1.5); }
.blueprint-socket.exec { width: 16px; height: 16px; border: 0; border-radius: 0; background: transparent; box-shadow: none; color: var(--socket-color); }
.blueprint-socket.exec svg { display: block; width: 16px; height: 16px; overflow: visible; filter: drop-shadow(0 1px 1px #000) drop-shadow(0 0 1px #ffffff99); }
.blueprint-socket.exec path { fill: var(--socket-fill); stroke: #ffffff; stroke-width: .75; stroke-linejoin: round; vector-effect: non-scaling-stroke; }
</style>
