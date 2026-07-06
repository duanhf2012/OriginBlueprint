<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = defineProps<{ data: { value?: unknown; type?: 'text' | 'number'; itemType?: 'string' | 'number'; setValue: (value: unknown) => void } }>()
const value = ref<unknown>(Array.isArray(props.data.value) ? [...props.data.value] : props.data.value ?? '')
const isArray = computed(() => Array.isArray(value.value))
const isBoolean = computed(() => typeof value.value === 'boolean')
const scalarInputType = computed(() => props.data.type === 'number' ? 'number' : 'text')

watch(value, next => { props.data.setValue(next); document.dispatchEvent(new CustomEvent('origin-control-change')) }, { deep: true })

function addItem() {
  if (!Array.isArray(value.value)) return
  value.value.push(props.data.itemType === 'number' ? 0 : '')
}

function removeItem(index: number) {
  if (Array.isArray(value.value)) value.value.splice(index, 1)
}

function updateItem(index: number, event: Event) {
  if (!Array.isArray(value.value)) return
  const next = (event.target as HTMLInputElement).value
  value.value[index] = props.data.itemType === 'number' ? Number(next) : next
}

function updateScalar(event: Event) {
  const next = (event.target as HTMLInputElement).value
  value.value = props.data.type === 'number' ? (next === '' ? '' : Number(next)) : next
}

</script>

<template>
  <label v-if="isBoolean" class="boolean-control" @pointerdown.stop @dblclick.stop><input v-model="value" type="checkbox" /><span>{{ value ? 'True' : 'False' }}</span></label>
  <div v-else-if="isArray" class="array-control" @pointerdown.stop @dblclick.stop>
    <div v-for="(item, index) in (value as Array<unknown>)" :key="index" class="array-item"><input :value="item" :type="data.itemType === 'number' ? 'number' : 'text'" @input="updateItem(index, $event)" /><button @click="removeItem(index)">×</button></div>
    <button class="array-add" @click="addItem">＋ Item</button>
  </div>
  <input v-else :value="value" :type="scalarInputType" class="node-input" @pointerdown.stop @dblclick.stop @input="updateScalar" />
</template>

<style scoped>
.node-input { width: 58px; height: 20px; padding: 1px 5px; border: 1px solid #777; border-radius: 2px; outline: 0; background: #f3f3f3; color: #171717; font: var(--node-control-font-size, 12px) Consolas, monospace; }
.node-input:focus { border-color: #53a5db; box-shadow: 0 0 0 1px #53a5db; }
.boolean-control { display: flex; align-items: center; gap: 3px; color: #f45a63; font: var(--node-badge-font-size, 10px) Consolas, monospace; }.boolean-control input { width: 14px; height: 14px; accent-color: #d83440; }
.array-control { width: 112px; padding: 3px; border: 1px solid #555; border-radius: 2px; background: #222; }.array-item { display: flex; margin-bottom: 2px; }.array-item input { min-width: 0; width: 84px; height: 19px; border: 1px solid #666; background: #eee; color: #111; font: var(--node-control-font-size, 11px) Consolas, monospace; }.array-item button, .array-add { border: 1px solid #555; background: #333; color: #aaa; font-size: var(--node-badge-font-size, 10px); }.array-item button { width: 21px; padding: 0; }.array-add { width: 100%; height: 20px; }
</style>
