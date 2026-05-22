<script setup lang="ts">
import type { Camera } from '~/types/camera'

const props = defineProps<{
  cameras: Camera[]
  focusedId: string | null
  latencyMs?: number
}>()

const emit = defineEmits<{ settingsToggle: [] }>()

const now = useClock()
const utc = computed(() => now.value.toISOString().slice(11, 19))
const online = computed(() => props.cameras.filter(c => c.online).length)
const total = computed(() => props.cameras.length)
const meshDot = computed(() => {
  if (online.value === total.value) return 'live'
  if (online.value === 0) return 'red'
  return 'warn'
})
</script>

<template>
  <header class="topbar">
    <!-- Brand -->
    <div class="brand">
      <svg width="22" height="22" viewBox="0 0 22 22" style="color: var(--ink)">
        <rect x="2" y="2" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.5"/>
        <rect x="6" y="6" width="10" height="10" fill="currentColor"/>
        <circle cx="11" cy="11" r="2" fill="var(--bg-deep)"/>
      </svg>
      <div class="brand-text">
        <div class="brand-name">SENTINEL <span class="brand-ver">// v0.4.2</span></div>
        <div class="brand-sub">ESP32 CAMERA MESH · OPS CONSOLE</div>
      </div>
    </div>

    <!-- Breadcrumbs -->
    <div class="breadcrumbs">
      <AppTopBarCrumb label="WORKSPACE" value="DEFAULT" />
      <span class="sep">›</span>
      <AppTopBarCrumb label="SITE" value="HQ · BUILDING-7" />
      <span class="sep">›</span>
      <AppTopBarCrumb label="VIEW" value="GRID ▾" />
      <template v-if="focusedId">
        <span class="sep">›</span>
        <AppTopBarCrumb label="FOCUS" :value="focusedId.toUpperCase()" accent />
      </template>
    </div>

      <!-- Status cluster -->
    <div class="status-cluster">
      <AppTopBarChip label="MESH" :value="`${online}/${total}`" :dot="meshDot" />
      <AppTopBarChip label="SOURCE" value="API" dot="accent" />
      <AppTopBarChip label="LATENCY" :value="`${latencyMs ?? 42}ms`" />
      <AppTopBarChip label="UTC" :value="utc" />
      <div class="rec-pill">
        <span class="rec-dot blink" />
        REC
      </div>
      <button class="settings-btn" title="Settings" @click="emit('settingsToggle')">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.2">
          <circle cx="7" cy="7" r="2.5"/>
          <path d="M7 1v1.5M7 11.5V13M1 7h1.5M11.5 7H13M2.93 2.93l1.06 1.06M10.01 10.01l1.06 1.06M2.93 11.07l1.06-1.06M10.01 3.99l1.06-1.06"/>
        </svg>
      </button>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  border-bottom: 1px solid var(--line);
  background: oklch(0.115 0.006 240 / .9);
  height: 48px;
  position: relative;
  z-index: 5;
  flex-shrink: 0;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  border-right: 1px solid var(--line);
  height: 100%;
}

.brand-text { display: flex; flex-direction: column; gap: 0; line-height: 1; }
.brand-name { font-family: var(--sans); font-weight: 600; font-size: 13px; letter-spacing: .02em; }
.brand-ver { color: var(--ink-faint); font-weight: 400; }
.brand-sub { font-size: 9px; color: var(--ink-faint); letter-spacing: .15em; margin-top: 3px; }

.breadcrumbs {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 0 18px;
  font-size: 11px;
  color: var(--ink-dim);
}

.sep { color: var(--ink-faint); }

.status-cluster {
  display: flex;
  align-items: stretch;
  height: 100%;
  border-left: 1px solid var(--line);
}

.rec-pill {
  display: flex;
  align-items: center;
  padding: 0 14px;
  color: var(--ink-faint);
  font-size: 10px;
  letter-spacing: .15em;
  gap: 8px;
  border-left: 1px solid var(--line);
}

.rec-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--live);
}

.settings-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 14px;
  border: none;
  border-left: 1px solid var(--line);
  background: transparent;
  color: var(--ink-faint);
  cursor: pointer;
  transition: color .15s, background .15s;
}
.settings-btn:hover { background: var(--panel-hi); color: var(--ink); }
</style>
