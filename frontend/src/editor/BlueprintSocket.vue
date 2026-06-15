<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ data: { name: string } }>()
const isExec = computed(() => props.data.name === 'exec')
const socketClass = computed(() => `socket-${props.data.name.replace(/[^a-z0-9_-]/gi, '-').toLowerCase()}`)
</script>

<template>
  <div class="blueprint-socket" :class="[socketClass, { exec: isExec }]" :title="data.name">
    <svg v-if="isExec" viewBox="0 0 14 16" aria-hidden="true">
      <path d="M2 2.5H5.2L12 8L5.2 13.5H2" />
    </svg>
  </div>
</template>

<style scoped>
.blueprint-socket { width: 11px; height: 11px; border: 2px solid #a8a8a8; border-radius: 50%; background: #161616; cursor: crosshair; transition: transform .1s, filter .1s; }
.blueprint-socket:hover { transform: scale(1.25); filter: brightness(1.5); }
.blueprint-socket.exec { width: 14px; height: 16px; border: 0; border-radius: 0; background: transparent; }
.blueprint-socket.exec svg { display: block; width: 14px; height: 16px; overflow: visible; }
.blueprint-socket.exec path { fill: none; stroke: #f0f0f0; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; vector-effect: non-scaling-stroke; }
.socket-integer, .socket-number { border-color: #20bd3d; }.socket-float { border-color: #4fc4a5; }.socket-boolean { border-color: #f12b38; }.socket-string { border-color: #e725b8; }.socket-array { border-color: #d6ac19; }.socket-file { border-color: #84b6e0; }.socket-table { border-color: #47c99d; }.socket-dictionary { border-color: #d98945; }.socket-any { border-color: #3a91db; }
</style>
