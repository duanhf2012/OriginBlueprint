export const socketThemes = {
  exec: { color: '#f2f2f2', fill: '#f4f4f4', label: '#f2f2f2' },
  integer: { color: '#21c46b' },
  number: { color: '#21c46b' },
  float: { color: '#8aff3d' },
  boolean: { color: '#d00000', fill: '#180000', label: '#ff3333' },
  string: { color: '#d85cff' },
  array: { color: '#f0c230' },
  any: { color: '#00a8e8', label: '#55bfff' }
} as const

export type SocketThemeName = keyof typeof socketThemes

export function normalizeSocketName(name?: string): SocketThemeName {
  const value = String(name ?? 'any').toLowerCase()
  return value in socketThemes ? value as SocketThemeName : 'any'
}

export function socketClassName(name?: string) {
  return `socket-${normalizeSocketName(name)}`
}

export function socketStyle(name?: string) {
  const theme = socketThemes[normalizeSocketName(name)]
  const color = theme.color
  return {
    '--socket-color': color,
    '--socket-fill': 'fill' in theme ? theme.fill : color,
    '--socket-label-color': 'label' in theme ? theme.label : color,
    '--connection-color': color
  }
}
