import { lookupConfigName } from './configTables'

// 与画布节点一致的文本宽度估算：中文/全角 12px，其余 7px；用于引用控件按内容自适应加宽。
export function estimateTextWidth(value: string | undefined, min = 0, max = 220): number {
  let width = 0
  for (const char of String(value ?? '')) width += /[\u4e00-\u9fff\uff00-\uffef]/.test(char) ? 12 : 7
  return Math.max(min, Math.min(max, Math.ceil(width)))
}

// 引用控件静态显示的长度上限：14 个中文字宽（168px），超出截断加省略号。
export const REF_DISPLAY_MAX_WIDTH = 14 * 12

// 按视觉宽度截断文本：超预算时回退给省略号（…，12px）留位，保证省略号一定显示。
export function clipTextToWidth(text: string, maxWidth: number): string {
  const chars = Array.from(String(text ?? ''))
  let width = 0
  let cut = -1
  for (let i = 0; i < chars.length; i++) {
    const w = /[\u4e00-\u9fff\uff00-\uffef]/.test(chars[i]) ? 12 : 7
    if (width + w > maxWidth) { cut = i; break }
    width += w
  }
  if (cut < 0) return chars.join('')
  while (cut > 0 && width + 12 > maxWidth) {
    cut -= 1
    width -= /[\u4e00-\u9fff\uff00-\uffef]/.test(chars[cut]) ? 12 : 7
  }
  return chars.slice(0, cut).join('') + '…'
}

// 引用控件静态显示文本：解析成功显示「id:名称」（超长按 14 个中文字宽截断），否则回退原始 id；空值显示空。
export function refStaticDisplay(tableKey: string, value: unknown): string {
  const id = String(value ?? '')
  if (!id) return ''
  const name = lookupConfigName(tableKey, value)
  return clipTextToWidth(name ? `${id}:${name}` : id, REF_DISPLAY_MAX_WIDTH)
}
