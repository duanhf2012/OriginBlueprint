<script setup lang="ts">
import { computed } from 'vue'
import { socketClassName, socketStyle } from './socketTheme'

const props = defineProps<{ data: { name: string; filled?: boolean } }>()
const isExec = computed(() => props.data.name === 'exec')
const isCallback = computed(() => props.data.name === 'callback')
const socketClass = computed(() => socketClassName(props.data.name))
const styleVariables = computed(() => socketStyle(props.data.name))
</script>

<template>
  <div class="blueprint-socket" :class="[socketClass, { exec: isExec, callback: isCallback, filled: data.filled }]" :style="styleVariables" :title="data.name">
    <svg v-if="isExec" viewBox="0 0 16 16" aria-hidden="true">
      <path d="M2.6 2.2H7.2L13.6 8L7.2 13.8H2.6Z" />
    </svg>
	<svg v-else-if="isCallback" viewBox="0 0 18 18" aria-hidden="true">
	  <path d="M9 1.5 16.5 9 9 16.5 1.5 9Z" />
	  <circle cx="9" cy="9" r="2.1" />
	</svg>
  </div>
</template>

<style scoped>
.blueprint-socket { width: 13px; height: 13px; box-sizing: border-box; border: 2px solid var(--socket-color); border-radius: 50%; background: #101010; box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff24; cursor: crosshair; transition: transform .1s, filter .1s; }
.blueprint-socket.filled { background: var(--socket-fill); box-shadow: 0 0 0 1px #000, inset 0 1px #ffffff55, 0 0 4px var(--socket-color); }
.blueprint-socket:hover { transform: scale(1.25); filter: brightness(1.5); }
.blueprint-socket.exec { width: 16px; height: 16px; border: 0; border-radius: 0; background: transparent; box-shadow: none; color: var(--socket-color); }
.blueprint-socket.exec svg { display: block; width: 16px; height: 16px; overflow: visible; filter: drop-shadow(0 1px 1px #000) drop-shadow(0 0 1px #ffffff99); }
.blueprint-socket.exec path { fill: #101010; stroke: #ffffff; stroke-width: 1.1; stroke-linejoin: round; vector-effect: non-scaling-stroke; }
.blueprint-socket.exec.filled path { fill: var(--socket-fill); stroke-width: .75; }
.blueprint-socket.callback { width: 18px; height: 18px; border: 0; border-radius: 0; background: transparent; box-shadow: none; color: var(--socket-color); }
.blueprint-socket.callback svg { display: block; width: 18px; height: 18px; overflow: visible; filter: drop-shadow(0 1px 1px #000) drop-shadow(0 0 2px #ff9f2f88); }
.blueprint-socket.callback path { fill: #2d1b08; stroke: var(--socket-color); stroke-width: 1.5; stroke-linejoin: round; }
.blueprint-socket.callback circle { fill: var(--socket-color); }
.blueprint-socket.callback.filled path { fill: var(--socket-fill); }
.blueprint-socket.callback.filled circle { fill: #38200a; }
</style>
