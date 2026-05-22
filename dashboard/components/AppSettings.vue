<script setup lang="ts">
import type { DashboardSettings } from '~/types/camera'

const props = defineProps<{ settings: DashboardSettings }>()
const emit = defineEmits<{
  'update:settings': [s: DashboardSettings]
  close: []
}>()

function set<K extends keyof DashboardSettings>(key: K, value: DashboardSettings[K]) {
  emit('update:settings', { ...props.settings, [key]: value })
}

const ACCENT_OPTIONS = ['#7cb9ff', '#ff6b6b', '#7ce0a6', '#e0a87c', '#c9a0ff']
</script>

<template>
  <div class="settings-panel">
    <div class="sp-header">
      <span class="sp-title">SETTINGS</span>
      <button class="sp-close" @click="emit('close')">✕</button>
    </div>

    <div class="sp-body">
      <!-- Layout section -->
      <div class="sp-section">LAYOUT</div>
      <div class="sp-row sp-row--h">
        <span class="sp-label">Grid density</span>
        <div class="seg-ctrl">
          <button
            v-for="d in ['comfy', 'regular', 'compact']"
            :key="d"
            :class="['seg-btn', { 'seg-btn--active': settings.density === d }]"
            @click="set('density', d as DashboardSettings['density'])"
          >{{ d.toUpperCase() }}</button>
        </div>
      </div>
      <div class="sp-row sp-row--h">
        <span class="sp-label">Camera HUD</span>
        <button
          :class="['toggle', { 'toggle--on': settings.overlays }]"
          @click="set('overlays', !settings.overlays)"
        ><i /></button>
      </div>
      <div class="sp-row sp-row--h">
        <span class="sp-label">Focus pane</span>
        <button
          :class="['toggle', { 'toggle--on': settings.showFocus }]"
          @click="set('showFocus', !settings.showFocus)"
        ><i /></button>
      </div>
      <div class="sp-row sp-row--h">
        <span class="sp-label">Scanlines</span>
        <button
          :class="['toggle', { 'toggle--on': settings.scanlines }]"
          @click="set('scanlines', !settings.scanlines)"
        ><i /></button>
      </div>

      <!-- Theme section -->
      <div class="sp-section">THEME</div>
      <div class="sp-row">
        <span class="sp-label">Accent color</span>
        <div class="color-swatches">
          <button
            v-for="c in ACCENT_OPTIONS"
            :key="c"
            :class="['swatch', { 'swatch--active': settings.accent === c }]"
            :style="{ background: c }"
            :title="c"
            @click="set('accent', c)"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings-panel {
  position: fixed;
  right: 16px;
  top: 64px;
  width: 280px;
  background: oklch(0.18 0.008 240 / .97);
  border: 1px solid var(--line-hi);
  z-index: 100;
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 80px);
  overflow: hidden;
}

.sp-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-bottom: 1px solid var(--line);
}
.sp-title { font-size: 11px; font-weight: 600; letter-spacing: .12em; color: var(--ink); }
.sp-close {
  width: 22px; height: 22px;
  border: 1px solid var(--line);
  background: transparent;
  color: var(--ink-dim);
  cursor: pointer;
  font-size: 11px;
  transition: background .12s, color .12s;
}
.sp-close:hover { background: var(--panel-hi); color: var(--ink); }

.sp-body {
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  overflow-y: auto;
}

.sp-section {
  font-size: 9px;
  font-weight: 600;
  letter-spacing: .15em;
  color: var(--ink-faint);
  margin-top: 4px;
}
.sp-section:first-child { margin-top: 0; }

.sp-row { display: flex; flex-direction: column; gap: 6px; }
.sp-row--h { flex-direction: row; align-items: center; justify-content: space-between; }
.sp-label { font-size: 11px; color: var(--ink-dim); }
.sp-hint { font-size: 9px; color: var(--ink-faint); margin: -4px 0 0; line-height: 1.5; }

.seg-ctrl { display: flex; gap: 2px; }
.seg-btn {
  padding: 3px 10px;
  font: inherit;
  font-size: 10px;
  letter-spacing: .1em;
  background: transparent;
  border: 1px solid var(--line);
  color: var(--ink-faint);
  cursor: pointer;
  transition: background .1s, color .1s, border-color .1s;
}
.seg-btn--active {
  background: var(--panel-hi);
  border-color: var(--ink-dim);
  color: var(--ink);
}
.seg-btn:hover:not(.seg-btn--active) { border-color: var(--line-hi); color: var(--ink-dim); }

.toggle {
  position: relative;
  width: 32px;
  height: 18px;
  border: none;
  border-radius: 999px;
  background: var(--line-hi);
  cursor: pointer;
  padding: 0;
  transition: background .15s;
  flex-shrink: 0;
}
.toggle--on { background: var(--live); }
.toggle i {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  transition: transform .15s;
  pointer-events: none;
}
.toggle--on i { transform: translateX(14px); }

.color-swatches { display: flex; gap: 6px; flex-wrap: wrap; }
.swatch {
  width: 24px;
  height: 24px;
  border: 2px solid transparent;
  border-radius: 3px;
  cursor: pointer;
  padding: 0;
  transition: border-color .1s, transform .1s;
}
.swatch--active { border-color: var(--ink); transform: scale(1.1); }
.swatch:hover:not(.swatch--active) { border-color: var(--ink-dim); }
</style>
