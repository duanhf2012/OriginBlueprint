<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { isIntegerInputDraft, isValidIntegerDefault, normalizeIntegerInput, parseIntegerInput } from './valueValidation'
import { lookupConfigName, searchConfigEntries, configTableLabel } from './configTables'
import { estimateTextWidth, refStaticDisplay } from './textWidth'

const props = defineProps<{ data: { value?: unknown; type?: 'text' | 'number'; integer?: boolean; itemType?: 'string' | 'number'; tableKey?: string; setValue: (value: unknown) => void } }>()
const value = ref<unknown>(Array.isArray(props.data.value) ? [...props.data.value] : props.data.value ?? '')
const isArray = computed(() => Array.isArray(value.value))
const isBoolean = computed(() => typeof value.value === 'boolean')
const isRefSelect = computed(() => typeof props.data.tableKey === 'string' && props.data.tableKey !== '')
const scalarInputType = computed(() => props.data.type === 'number' && !props.data.integer ? 'number' : 'text')
const isTextScalar = computed(() => !props.data.integer && props.data.type !== 'number')
const scalarIntegerInvalid = ref(Boolean(props.data.integer && value.value !== '' && !isValidIntegerDefault(value.value)))

const refOpen = ref(false)
const refQuery = ref('')
// 下拉最多展示的条数；同虚幻 SComboBox 打开即全列表（截断到上限），输入后按查询过滤。
const REF_DROPDOWN_LIMIT = 30
const refMatches = computed(() => {
  if (!isRefSelect.value) return []
  // 空查询（未编辑或删除到空）一律展示全量条目，从第一行（通常是 id 0）开始；
  // 当前值若落在截断区之外则换入末位，保证选中项始终可见。
  const query = refEdited() ? refQuery.value.trim() : ''
  if (!query) {
    const entries = searchConfigEntries(props.data.tableKey ?? '', '', REF_DROPDOWN_LIMIT + 1)
    const currentId = String(value.value ?? '')
    if (!currentId || entries.length <= REF_DROPDOWN_LIMIT || entries.some(entry => entry.id === currentId)) {
      return entries.slice(0, REF_DROPDOWN_LIMIT)
    }
    return entries.slice(0, REF_DROPDOWN_LIMIT - 1).concat(entries.filter(entry => entry.id === currentId).slice(0, 1))
  }
  return searchConfigEntries(props.data.tableKey ?? '', query, REF_DROPDOWN_LIMIT)
})
const refResolvedName = computed(() => (isRefSelect.value ? lookupConfigName(props.data.tableKey ?? '', value.value) : undefined))
const refUnknown = computed(() => isRefSelect.value && String(value.value ?? '') !== '' && !refResolvedName.value)
// 显示：解析成功常显「id:备注名」（id 前置便于对表，同 Unreal 行引用的可读行名）；未命中或表未加载回退显示数字 id。
// 文档存档始终是原始 id，显示仅用于阅读。
const refClosedDisplay = computed(() => refStaticDisplay(props.data.tableKey ?? '', value.value))
const refDisplay = computed(() => {
  if (refOpen.value) return refQuery.value
  return refClosedDisplay.value
})
// 输入框宽度按静态显示文本自适应（108px 起步、220px 封顶，超长截断由悬停提示兜底）；搜索态不随输入抖动。
const refInputWidthStyle = computed(() => ({ width: `${estimateTextWidth(refClosedDisplay.value, 108, 220)}px` }))
const refTitle = computed(() => {
  if (!isRefSelect.value) return undefined
  const label = configTableLabel(props.data.tableKey ?? '')
  const id = String(value.value ?? '')
  if (!id) return `从数据集 ${label} 搜索选择（支持 ID 范围如 100-200、比较如 >1000），或直接输入 ID`
  if (refResolvedName.value) return `${value.value ?? ''} · ${refResolvedName.value}（数据集 ${label}）`
  return `未知 ID（数据集 ${label} 中未找到）`
})

// 聚焦进入搜索态时以解析出的名称为种子，显示不从中文名跳回数字 id；未解析到名称才回退原始 id。
const refSeed = ref('')

function focusRef(event: FocusEvent) {
  // 种子与静态显示一致（id:名称），聚焦只加全选高亮不跳变内容。
  refSeed.value = refClosedDisplay.value
  refQuery.value = refSeed.value
  refActiveIndex.value = null
  refOpen.value = true
  // 同虚幻可编辑引脚 SelectAllTextWhenFocused：聚焦即全选种子，直接输入 id/文字可整体替换。
  const input = event.target as HTMLInputElement
  void nextTick(() => input.select())
}

function refEdited() {
  return refQuery.value !== refSeed.value
}

const refRawId = computed(() => String(value.value ?? ''))

// 打开下拉时把当前选中行滚到列表中部（只动列表自身滚动，不触发画布平移）。
const refOptionsEl = ref<HTMLElement | null>(null)
watch(refOpen, open => {
  if (!open) return
  void nextTick(() => {
    const list = refOptionsEl.value
    const selected = list?.querySelector<HTMLElement>('.ref-option.selected')
    if (list && selected) list.scrollTop = Math.max(0, selected.offsetTop - (list.clientHeight - selected.offsetHeight) / 2)
  })
})

function inputRef(event: Event) {
  refQuery.value = (event.target as HTMLInputElement).value
  refActiveIndex.value = null
  // 删除到空回到全量列表时，默认高亮第一行（通常是 id 0），回车即选中该行。
  if (!refQuery.value.trim() && refMatches.value.length) refActiveIndex.value = 0
}

function commitRef(next: string) {
  const trimmed = next.trim()
  if (trimmed === String(value.value ?? '')) {
    refOpen.value = false
    return
  }
  // 整名精确命中某行时按该行 id 提交，避免把行名原文写进档（搜索态种子与输入都可能是名称）。
  const exact = searchConfigEntries(props.data.tableKey ?? '', trimmed).find(entry => entry.name === trimmed)
  if (exact && exact.id !== trimmed) {
    commitRef(exact.id)
    return
  }
  // 「id:名称」形式的输入按前缀 id 提交（静态显示就是该格式，直接改前缀数字即可换行）。
  const prefixed = trimmed.match(/^(\d+)\s*[:：]/)
  if (prefixed && prefixed[1] !== trimmed) {
    commitRef(prefixed[1])
    return
  }
  beginEdit()
  if (props.data.integer) {
    const parsed = parseIntegerInput(trimmed)
    value.value = parsed !== undefined ? parsed : trimmed
  } else {
    value.value = trimmed
  }
  commitEdit()
  refOpen.value = false
}

// ↑/↓ 在下拉中移动键盘高亮（从当前值行起步，循环滚动），回车选中高亮项（同虚幻 SComboBox 键盘操作）。
// 未用方向键时回车保持原语义：未编辑只收起、编辑过按输入文本提交。
const refActiveIndex = ref<number | null>(null)

function moveActiveRef(delta: number) {
  const count = refMatches.value.length
  if (!count) return
  const currentRow = refActiveIndex.value ?? Math.max(0, refMatches.value.findIndex(entry => entry.id === String(value.value ?? '')))
  refActiveIndex.value = (currentRow + delta + count) % count
  void nextTick(() => {
    const list = refOptionsEl.value
    const active = list?.querySelector<HTMLElement>('.ref-option.active')
    if (list && active) list.scrollTop = Math.max(0, active.offsetTop - (list.clientHeight - active.offsetHeight) / 2)
  })
}

function keydownRef(event: KeyboardEvent) {
  // 输入法组合期间的按键（含回车确认候选）不拦截、不提交，避免把组字中的文本写进档。
  if (event.isComposing) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (refOpen.value) moveActiveRef(event.key === 'ArrowDown' ? 1 : -1)
    return
  }
  if (event.key === 'Enter') {
    event.preventDefault()
    if (refActiveIndex.value !== null) {
      const entry = refMatches.value[refActiveIndex.value]
      if (entry) pickRef(entry.id)
      else refOpen.value = false
      return
    }
    // 未改动种子（仍是当前行的名称/id）时回车只收起，不改值。
    if (refEdited()) commitRef(refQuery.value)
    else {
      refOpen.value = false
      ;(event.target as HTMLInputElement).blur()
    }
  } else if (event.key === 'Escape') {
    refOpen.value = false
    ;(event.target as HTMLInputElement).blur()
  }
}

function pickRef(id: string) {
  commitRef(id)
}

function blurRef() {
  window.setTimeout(() => {
    if (!refOpen.value) return
    refOpen.value = false
    // 失焦即提交输入的原始 ID（含 Ctrl+V 粘贴），与普通输入框一致；Esc 取消时 refOpen 已为 false。
    // 未改动过种子（仍是名称占位）则丢弃，不改值。
    if (!refEdited()) return
    const trimmed = refQuery.value.trim()
    if (trimmed && trimmed !== String(value.value ?? '')) commitRef(trimmed)
  }, 120)
}

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
  <div v-else-if="isRefSelect" class="ref-select-control" :class="{ unknown: refUnknown }" @pointerdown.stop @dblclick.stop>
    <input class="node-input ref-input" :class="{ seeded: refOpen && !refEdited(), resolved: !refOpen && !!refResolvedName }" :style="refInputWidthStyle" :value="refDisplay" :title="refTitle" type="text" placeholder="搜索/ID范围" @focus="focusRef" @input="inputRef" @keydown="keydownRef" @blur="blurRef" />
    <div v-if="refOpen" ref="refOptionsEl" class="ref-options">
      <button v-for="(entry, entryIndex) in refMatches" :key="entry.id" type="button" class="ref-option" :class="{ selected: entry.id === String(value), active: entryIndex === refActiveIndex }" @pointerdown.prevent @click="pickRef(entry.id)"><span class="ref-option-id">{{ entry.id }}</span><span class="ref-option-name">{{ entry.name }}</span></button>
      <div v-if="refMatches.length >= REF_DROPDOWN_LIMIT" class="ref-options-cap">仅显示前 {{ REF_DROPDOWN_LIMIT }} 条，输入文字可筛选</div>
      <button type="button" class="ref-option ref-apply-raw" @pointerdown.prevent @click="commitRef(refEdited() ? refQuery : refRawId)"><span class="ref-option-id">{{ (refEdited() ? refQuery : refRawId) || '（空）' }}</span><span class="ref-option-name">使用输入的原始 ID</span></button>
    </div>
  </div>
  <input v-else :value="value" :type="scalarInputType" :inputmode="data.integer ? 'numeric' : undefined" :pattern="data.integer ? '[+-]?[0-9]*' : undefined" :aria-invalid="data.integer ? scalarIntegerInvalid : undefined" :title="scalarIntegerInvalid ? '请输入 64 位整数' : undefined" class="node-input" :class="{ 'string-input': isTextScalar, 'invalid-integer': scalarIntegerInvalid }" @pointerdown.stop @dblclick.stop @focus="beginScalarEdit" @blur="commitScalarEdit" @beforeinput="guardIntegerBeforeInput" @paste="guardIntegerPaste" @input="updateScalar" />
</template>

<style scoped>
.node-input { width: 58px; height: 20px; padding: 1px 5px; border: 1px solid #777; border-radius: 2px; outline: 0; background: #f3f3f3; color: #171717; font: var(--node-control-font-size, 12px) Consolas, monospace; }
.node-input.string-input { width: 92px; }
.node-input:focus { border-color: #53a5db; box-shadow: 0 0 0 1px #53a5db; }
.node-input.invalid-integer { border-color: #e04f5f; box-shadow: 0 0 0 1px #e04f5f; }
.boolean-control { display: flex; align-items: center; gap: 3px; color: #f45a63; font: var(--node-badge-font-size, 10px) Consolas, monospace; }.boolean-control input { width: 14px; height: 14px; accent-color: #d83440; }
.array-control { width: 112px; padding: 3px; border: 1px solid #555; border-radius: 2px; background: #222; }.array-item { display: flex; margin-bottom: 2px; }.array-item input { min-width: 0; width: 84px; height: 19px; border: 1px solid #666; background: #eee; color: #111; font: var(--node-control-font-size, 11px) Consolas, monospace; }.array-item button, .array-add { border: 1px solid #555; background: #333; color: #aaa; font-size: var(--node-badge-font-size, 10px); }.array-item button { width: 21px; padding: 0; }.array-add { width: 100%; height: 20px; }
.ref-select-control { position: relative; }
.ref-select-control .ref-input { width: 108px; }
/* 搜索态的名称种子是系统代填的占位内容，用灰色与用户输入的文字区分（输入后恢复常规色）。 */
.ref-select-control .ref-input.seeded { color: #6f7d88; }
/* 静态显示中：解析出的名称用 UE int 引脚同族的高饱和青色+半粗体（与近黑的手输原始 id 保持明显色相差），
   手输的原始 id 保持常规黑色，一眼可分。 */
.ref-select-control .ref-input.resolved { color: #007e8f; font-weight: 600; }
.ref-select-control.unknown .ref-input { border-color: #e04f5f; box-shadow: 0 0 0 1px #e04f5f; }
.ref-options { position: absolute; z-index: 12; top: calc(100% + 2px); right: 0; width: 190px; display: grid; max-height: 168px; overflow: auto; border: 1px solid #52636e; border-radius: 3px; background: #1d2326; box-shadow: 0 6px 16px #000a; }
.ref-options-cap { padding: 3px 7px; color: #7c8b96; font-size: var(--node-badge-font-size, 10px); }
.ref-option { display: flex; gap: 6px; align-items: baseline; min-height: 24px; padding: 3px 7px; overflow: hidden; border: 0; background: transparent; color: #dce6eb; text-align: left; cursor: pointer; }
.ref-option:hover, .ref-option.selected, .ref-option.active { background: #31566b; }
.ref-option-id { flex: 0 0 auto; color: #8fc1e3; font: 600 var(--node-control-font-size, 11px) Consolas, monospace; }
.ref-option-name { min-width: 0; overflow: hidden; color: #c3ccd3; font-size: var(--node-control-font-size, 11px); text-overflow: ellipsis; white-space: nowrap; }
.ref-apply-raw { border-top: 1px solid #36424a; }
.ref-apply-raw .ref-option-name { color: #91a8b8; font-style: italic; }
</style>
