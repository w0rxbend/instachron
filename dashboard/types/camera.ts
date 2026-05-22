export interface Camera {
  id: string
  index: number
  loc?: string
  res?: string
  targetFps?: number
  bitrate?: number
  signal?: number
  online: boolean
  uptime?: string
  fw?: string
  seed: number
  offlineMin?: number
}

export interface EventEntry {
  t: string
  lvl: 'OK' | 'INFO' | 'WARN' | 'ERR'
  src: string
  msg: string
}

export type GridDensity = 'comfy' | 'regular' | 'compact'

export interface DashboardSettings {
  density: GridDensity
  overlays: boolean
  accent: string
  showFocus: boolean
  scanlines: boolean
}
