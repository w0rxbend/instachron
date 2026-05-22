<script setup lang="ts">
import type { Camera } from '~/types/camera'

const props = defineProps<{
  cam: Camera
  accent: string
}>()

const emit = defineEmits<{ close: [] }>()

const { snapshotUrl, streamUrl } = useCameraApi()

// Deterministic histogram bars from seed
const bars = computed(() => {
  let s = (props.cam.seed || 1) * 7919
  return Array.from({ length: 48 }, () => {
    s = (s * 1664525 + 1013904223) >>> 0
    return 0.15 + (s / 0xffffffff) * 0.85
  })
})

function barColor(i: number) {
  if (i > 36) return 'var(--accent-2)'
  if (i > 24) return 'var(--accent)'
  return 'var(--ink-mute)'
}

function openSnapshot() {
  window.open(snapshotUrl(props.cam.id), '_blank')
}
</script>

<template>
  <div class="focus-pane">
    <!-- Header -->
    <div class="fp-header">
      <div>
        <div class="fp-sub">SELECTED CAMERA</div>
        <div class="fp-title">{{ cam.id.toUpperCase() }} · INDEX {{ cam.index }}</div>
      </div>
      <button class="fp-close" @click="emit('close')">✕</button>
    </div>

    <div class="fp-body">
      <!-- Endpoints -->
      <div class="section">
        <div class="section-title">
          <span>ENDPOINTS</span>
          <span class="section-rule" />
        </div>
        <AppFocusPaneKv k="STREAM" :v="streamUrl(cam.id)" mono />
        <AppFocusPaneKv k="SNAPSHOT" :v="snapshotUrl(cam.id)" mono />
      </div>

      <!-- Telemetry -->
      <div class="section">
        <div class="section-title">
          <span>TELEMETRY</span>
          <span class="section-rule" />
        </div>
        <AppFocusPaneKv k="STATUS">
          <span :style="{ color: cam.online ? 'var(--live)' : 'var(--accent-2)' }">
            {{ cam.online ? '● ONLINE' : '● OFFLINE' }}
          </span>
        </AppFocusPaneKv>
        <AppFocusPaneKv k="INDEX" :v="`${cam.index}`" />
        <AppFocusPaneKv k="RESOLUTION" :v="cam.res ?? '—'" />
        <AppFocusPaneKv k="TARGET FPS" :v="cam.targetFps != null ? `${cam.targetFps}` : '—'" />
        <AppFocusPaneKv k="BITRATE" :v="cam.bitrate != null ? `${cam.bitrate} kb/s` : '—'" />
        <AppFocusPaneKv k="SIGNAL" :v="cam.signal != null ? `${cam.signal}/4` : '—'" />
        <AppFocusPaneKv k="UPTIME" :v="cam.uptime ?? '—'" />
        <AppFocusPaneKv k="LOCATION" :v="cam.loc ?? '—'" />
        <AppFocusPaneKv k="FIRMWARE" :v="cam.fw ?? '—'" />
      </div>

      <!-- Actions -->
      <div class="section">
        <div class="section-title">
          <span>ACTIONS</span>
          <span class="section-rule" />
        </div>
        <button class="action-row" @click="openSnapshot">
          <span class="action-icon" style="color: var(--accent)">↗</span>
          <span class="action-label">Open snapshot</span>
          <span class="action-hint">GET {{ cam.id }}/snapshot</span>
        </button>
      </div>

      <!-- Frame histogram -->
      <div class="section">
        <div class="section-title">
          <span>FRAME HISTOGRAM</span>
          <span class="section-rule" />
        </div>
        <div class="histogram">
          <div
            v-for="(h, i) in bars"
            :key="i"
            class="histo-bar"
            :style="{ height: `${h * 100}%`, background: barColor(i) }"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.focus-pane {
  flex: 0 0 320px;
  border-left: 1px solid var(--line);
  background: oklch(0.13 0.006 240 / .95);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.fp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--line);
}
.fp-sub { font-size: 9px; color: var(--ink-faint); letter-spacing: .15em; }
.fp-title { font-size: 13px; color: var(--ink); letter-spacing: .04em; margin-top: 3px; font-weight: 600; }
.fp-close {
  width: 22px; height: 22px;
  border: 1px solid var(--line);
  background: transparent;
  color: var(--ink-dim);
  cursor: pointer;
  font-size: 11px;
  transition: background .12s, color .12s;
}
.fp-close:hover { background: var(--panel-hi); color: var(--ink); }

.fp-body {
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  overflow-y: auto;
  flex: 1;
}

.section { display: flex; flex-direction: column; gap: 6px; }
.section-title {
  font-size: 9px;
  color: var(--ink-faint);
  letter-spacing: .18em;
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 2px;
}
.section-rule { flex: 1; height: 1px; background: var(--line); }

.action-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border: 1px solid var(--line);
  background: transparent;
  color: var(--ink);
  font: inherit;
  font-size: 11px;
  cursor: pointer;
  text-align: left;
  transition: background .12s, border-color .12s;
}
.action-row:hover { background: var(--panel-hi); border-color: var(--line-hi); }
.action-icon { width: 14px; text-align: center; }
.action-label { flex: 1; }
.action-hint { font-size: 9px; color: var(--ink-faint); }

.histogram {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 64px;
  border: 1px solid var(--line);
  padding: 6px;
  background: var(--bg-deep);
}
.histo-bar { flex: 1; }
</style>
