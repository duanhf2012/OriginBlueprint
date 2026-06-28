import { enUS } from './en-US'
import { zhCN } from './zh-CN'

export const menuLocales = {
  'zh-CN': zhCN,
  'en-US': enUS
} as const

export type LocaleId = keyof typeof menuLocales

export function normalizeLocale(value: string | null | undefined): LocaleId {
  return value === 'en-US' ? 'en-US' : 'zh-CN'
}
