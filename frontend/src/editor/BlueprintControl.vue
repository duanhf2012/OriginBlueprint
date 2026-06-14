<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{ data: { value?: unknown; setValue: (value: unknown) => void } }>()
const value = ref(props.data.value ?? '')

watch(value, next => props.data.setValue(next))
</script>

<template>
  <input v-model="value" class="node-input" @pointerdown.stop @dblclick.stop />
</template>

<style scoped>
.node-input { width: 58px; height: 20px; padding: 1px 5px; border: 1px solid #777; border-radius: 2px; outline: 0; background: #f3f3f3; color: #171717; font: 12px Consolas, monospace; }
.node-input:focus { border-color: #53a5db; box-shadow: 0 0 0 1px #53a5db; }
</style>
