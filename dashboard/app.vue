<script setup lang="ts">
import type { Camera, DashboardSettings, EventEntry } from '~/types/camera'

const settings = reactive<DashboardSettings>({
  density: 'regular',
  overlays: true,
  accent: '#7cb9ff',
  showFocus: true,
  scanlines: true,
})

const { cameras } = useCameras()
const { snapshotUrl } = useCameraApi()

const focusedId = ref<string | null>(null)
const focusedCam = computed<Camera | undefined>(() =>
  cameras.value.find(c => c.id === focusedId.value)
)
const showSettings = ref(false)

function handleFocus(id: string) {
  focusedId.value = focusedId.value === id ? null : id
}

function handleSnapshot(cam: Camera) {
  window.open(snapshotUrl(cam.id), '_blank')
}

const gridCols = computed(() => {
  switch (settings.density) {
    case 'compact': return 4
    case 'comfy':   return 2
    default:        return 3
  }
})

const gridRowMinHeight = computed(() => {
  switch (settings.density) {
    case 'compact': return '180px'
    case 'comfy':   return '280px'
    default:        return '220px'
  }
})

watch(() => settings.accent, (v) => {
  if (!import.meta.client) return

  document.documentElement.style.setProperty('--accent', v)
}, { immediate: true })

const events: EventEntry[] = []
</script>

<template>
  <div class="dashboard">
    <AppTopBar
      :cameras="cameras"
      :focused-id="focusedId"
      @settings-toggle="showSettings = !showSettings"
    />

    <!-- Settings panel (floating) -->
    <Transition name="settings">
      <AppSettings
        v-if="showSettings"
        :settings="settings"
        @update:settings="Object.assign(settings, $event)"
        @close="showSettings = false"
      />
    </Transition>

    <!-- Main content row -->
    <div class="content-row">
      <AppSidePanel
        :cameras="cameras"
        :focused-id="focusedId"
        @focus="handleFocus"
      />

      <!-- Grid + event log column -->
      <main class="grid-col">
        <!-- Sub-toolbar -->
        <div class="subbar">
          <span class="subbar-title">LIVE GRID</span>
          <span class="subbar-sep">·</span>
          <span>{{ cameras.filter(c => c.online).length }} STREAMS · {{ cameras.filter(c => !c.online).length }} OFFLINE</span>
          <span class="subbar-spacer" />
          <button
            v-for="d in [['comfy', '1×2'], ['regular', '2×3'], ['compact', '3×4']] as const"
            :key="d[0]"
            :class="['grid-btn', { 'grid-btn--active': settings.density === d[0] }]"
            @click="settings.density = d[0]"
          >{{ d[1] }}</button>
          <span class="subbar-divider" />
          <button
            :class="['grid-btn', { 'grid-btn--active': settings.overlays }]"
            @click="settings.overlays = !settings.overlays"
          >HUD</button>
        </div>

        <!-- Camera grid -->
        <div class="grid-scroll">
          <div
            class="camera-grid"
            :style="{
              gridTemplateColumns: `repeat(${gridCols}, minmax(0, 1fr))`,
              gridAutoRows: `minmax(${gridRowMinHeight}, 1fr)`,
            }"
          >
            <CameraTile
              v-for="cam in cameras"
              :key="cam.id"
              :cam="cam"
              :focused="focusedId === cam.id"
              :overlays="settings.overlays"
              :accent="settings.accent"
              @focus="handleFocus"
              @snapshot="handleSnapshot"
            />
          </div>
        </div>

        <AppEventLog :events="events" />
      </main>

      <!-- Focus pane -->
      <Transition name="focus">
        <AppFocusPane
          v-if="focusedCam && settings.showFocus"
          :cam="focusedCam"
          :accent="settings.accent"
          @close="focusedId = null"
        />
      </Transition>
    </div>
  </div>
</template>

<style>
/* global reset for Nuxt */
#__nuxt { height: 100vh; overflow: hidden; }
</style>

<style scoped>
.dashboard {
  display: flex;
  flex-direction: column;
  height: 100vh;
  position: relative;
  overflow: hidden;
}

.content-row {
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.grid-col {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-deep);
  overflow: hidden;
}

/* Sub-toolbar */
.subbar {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--line);
  font-size: 10px;
  color: var(--ink-faint);
  letter-spacing: .15em;
  flex-shrink: 0;
}
.subbar-title { color: var(--ink); }
.subbar-sep { color: var(--ink-faint); }
.subbar-spacer { flex: 1; }
.subbar-divider { width: 1px; height: 14px; background: var(--line); }

.grid-btn {
  border: 1px solid var(--line);
  background: transparent;
  color: var(--ink-faint);
  font: inherit;
  font-size: 10px;
  letter-spacing: .12em;
  padding: 3px 8px;
  cursor: pointer;
  transition: background .1s, color .1s, border-color .1s;
}
.grid-btn--active {
  background: var(--panel-hi);
  border-color: var(--ink-dim);
  color: var(--ink);
}
.grid-btn:hover:not(.grid-btn--active) { border-color: var(--line-hi); color: var(--ink-dim); }

/* Camera grid */
.grid-scroll {
  flex: 1;
  min-height: 0;
  padding: 16px;
  overflow: auto;
}
.camera-grid {
  display: grid;
  gap: 12px;
  min-height: 100%;
}

/* Transitions */
.settings-enter-active,
.settings-leave-active { transition: opacity .15s, transform .15s; }
.settings-enter-from,
.settings-leave-to { opacity: 0; transform: translateY(-8px); }

.focus-enter-active,
.focus-leave-active { transition: opacity .15s, transform .15s; }
.focus-enter-from,
.focus-leave-to { opacity: 0; transform: translateX(16px); }
</style>
