<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { isIntegerInputDraft, isValidIntegerDefault, normalizeIntegerInput, parseIntegerInput } from './valueValidation'

const props = defineProps<{ data: { value?: unknown; type?: 'text' | 'number'; integer?: boolean; itemType?: 'string' | 'number'; setValue: (value: unknown) => void } }>()
const value = ref<unknown>(Array.isArray(props.data.value) ? [...props.data.value] : props.data.value ?? '')
const isArray = computed(() => Array.isArray(value.value))
const isBoolean = computed(() => typeof value.value === 'boolean')
const scalarInputType = computed(() => props.data.type === 'number' && !props.data.integer ? 'number' : 'text')
const scalarIntegerInvalid = ref(Boolean(props.data.integer && value.value !== '' && !isValidIntegerDefault(value.value)))

function beginEdit() {
  document.dispatchEvent(new CustomEvent('origin-control-edit-start'))
}

function commitEdit() {
  void nextTick(() => document.dispatchEvent(new CustomEvent('origin-control-edit-commit')))
}

watch(value, next => { props.data.setValue(next); document.dispatchEvent(new CustomEvent('origin-control-change')) }, { deep: true })

function addItem() {
  if (!Array.isArray(value.value)) return
  beginEdit()
  value.value.push(props.data.itemType === 'number' ? 0 : '')
  commitEdit()
}

function removeItem(index: number) {
  if (!Array.isArray(value.value)) return
  beginEdit()
  value.value.splice(index, 1)
  commitEdit()
}

function updateItem(index: number, event: Event) {
  if (!Array.isArray(value.value)) return
  const next = (event.target as HTMLInputElement).value
  value.value[index] = props.data.itemType === 'number' ? normalizeIntegerInput(next) : next
}

function updateScalar(event: Event) {
  const next = (event.target as HTMLInputElement).value
  if (props.data.integer) {
    const parsed = parseIntegerInput(next)
    scalarIntegerInvalid.value = !isIntegerInputDraft(next)
    if (parsed !== undefined) value.value = parsed
    return
  }
  value.value = props.data.type === 'number' ? (next === '' ? '' : Number(next)) : next
}

function integerEditCandidate(input: HTMLInputElement, inserted: string) {
  const start = input.selectionStart ?? input.value.length
  const end = input.selectionEnd ?? start
  return input.value.slice(0, start) + inserted + input.value.slice(end)
}

function guardIntegerBeforeInput(event: Event) {
  const inputEvent = event as InputEvent
  if (!props.data.integer || inputEvent.inputType.startsWith('delete') || inputEvent.data === null) return
  if (!isIntegerInputDraft(integerEditCandidate(inputEvent.target as HTMLInputElement, inputEvent.data))) inputEvent.preventDefault()
}

function guardIntegerPaste(event: ClipboardEvent) {
  if (!props.data.integer) return
  const pasted = event.clipboardData?.getData('text') ?? ''
  if (!isIntegerInputDraft(integerEditCandidate(event.target as HTMLInputElement, pasted))) event.preventDefault()
}

function beginScalarEdit(event: FocusEvent) {
  beginEdit()
  if (props.data.integer && scalarIntegerInvalid.value) void nextTick(() => (event.target as HTMLInputElement).select())
}

function commitScalarEdit(event: FocusEvent) {
  if (props.data.integer) {
    const input = event.target as HTMLInputElement
    input.value = String(value.value ?? '')
    scalarIntegerInvalid.value = value.value !== '' && !isValidIntegerDefault(value.value)
  }
  commitEdit()
}

</script>

<template>
  <label v-if="isBoolean" class="boolean-control" @pointerdown.stop="beginEdit" @dblclick.stop><input v-model="value" type="checkbox" @change="commitEdit" /><span>{{ value ? 'True' : 'False' }}</span></label>
  <div v-else-if="isArray" class="array-control" @pointerdown.stop @dblclick.stop>
    <div v-for="(item, index) in (value as Array<unknown>)" :key="index" class="array-item"><input :value="item" type="text" :inputmode="data.itemType === 'number' ? 'numeric' : undefined" @focus="beginEdit" @blur="commitEdit" @input="updateItem(index, $event)" /><button @click="removeItem(index)">×</button></div>
    <button class="array-add" @click="addItem">＋ Item</button>
  </div>
  <input v-else :value="value" :type="scalarInputType" :inputmode="data.integer ? 'numeric' : undefined" :pattern="data.integer ? '[+-]?[0-9]*' : undefined" :aria-invalid="data.integer ? scalarIntegerInvalid : undefined" :title="scalarIntegerInvalid ? '请输入 64 位整数' : undefined" class="node-input" :class="{ 'invalid-integer': scalarIntegerInvalid }" @pointerdown.stop @dblclick.stop @focus="beginScalarEdit" @blur="commitScalarEdit" @beforeinput="guardIntegerBeforeInput" @paste="guardIntegerPaste" @input="updateScalar" />
</template>

<style scoped>
.node-input { width: 58px; height: 20px; padding: 1px 5px; border: 1px solid #777; border-radius: 2px; outline: 0; background: #f3f3f3; color: #171717; font: var(--node-control-font-size, 12px) Consolas, monospace; }
.node-input:focus { border-color: #53a5db; box-shadow: 0 0 0 1px #53a5db; }
.node-input.invalid-integer { border-color: #e04f5f; box-shadow: 0 0 0 1px #e04f5f; }
.boolean-control { display: flex; align-items: center; gap: 3px; color: #f45a63; font: var(--node-badge-font-size, 10px) Consolas, monospace; }.boolean-control input { width: 14px; height: 14px; accent-color: #d83440; }
.array-control { width: 112px; padding: 3px; border: 1px solid #555; border-radius: 2px; background: #222; }.array-item { display: flex; margin-bottom: 2px; }.array-item input { min-width: 0; width: 84px; height: 19px; border: 1px solid #666; background: #eee; color: #111; font: var(--node-control-font-size, 11px) Consolas, monospace; }.array-item button, .array-add { border: 1px solid #555; background: #333; color: #aaa; font-size: var(--node-badge-font-size, 10px); }.array-item button { width: 21px; padding: 0; }.array-add { width: 100%; height: 20px; }
</style>
