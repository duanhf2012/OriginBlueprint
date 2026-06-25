import type { App } from 'vue'
import { platform } from './platform'

function errorMessage(value: unknown) {
  if (value instanceof Error) return value.message
  if (typeof value === 'string') return value
  try { return JSON.stringify(value) } catch { return String(value) }
}

function errorStack(value: unknown) {
  return value instanceof Error ? value.stack ?? '' : ''
}

export function logFrontendError(context: string, value: unknown, stack = '') {
  const message = errorMessage(value)
  const trace = stack || errorStack(value)
  void platform.logClientError('error', message, trace, context)
}

export function installFrontendErrorLogging(app: App) {
  app.config.errorHandler = (error, instance, info) => {
    logFrontendError(`vue:${info}`, error)
    console.error(error)
  }
  window.addEventListener('error', event => {
    logFrontendError('window.error', event.error ?? event.message, event.error?.stack ?? '')
  })
  window.addEventListener('unhandledrejection', event => {
    logFrontendError('window.unhandledrejection', event.reason)
  })
}
