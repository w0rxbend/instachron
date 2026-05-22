<script setup lang="ts">
import type { Camera } from '~/types/camera'

const props = defineProps<{
  cam: Camera
  focused: boolean
  overlays: boolean
  accent: string
}>()

const emit = defineEmits<{
  focus: [id: string]
  snapshot: [cam: Camera]
}>()

const { streamUrl } = useCameraApi()
const pulse = ref(0)
let pulseId: ReturnType<typeof setInterval> | null = null

watch(() => props.cam.online, (online) => {
  if (pulseId) { clearInterval(pulseId); pulseId = null }
  if (!import.meta.client) return

  if (online) pulseId = setInterval(() => pulse.value++, 1000)
}, { immediate: true })
onUnmounted(() => { if (pulseId) clearInterval(pulseId) })

const fps = computed(() =>
  props.cam.online && props.cam.targetFps
    ? props.cam.targetFps - (pulse.value % 3 === 0 ? 1 : 0)
    : 0
)

const ts = computed(() => {
  const base = props.cam.online ? Date.now() : Date.now() - 1000 * 60 * (props.cam.offlineMin ?? 0)
  return new Date(base).toISOString().slice(11, 19) + 'Z'
})
</script>

<template>
  <div
    :class="['tile', { 'tile--focused': focused }]"
    @click="emit('focus', cam.id)"
    @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--line-hi)'"
    @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = focused ? 'var(--ink-dim)' : 'var(--line)'"
  >
    <!-- Feed area -->
    <div class="feed-area">
      <!-- Live MJPEG -->
      <img
        v-if="cam.online"
        :src="streamUrl(cam.id)"
        :alt="cam.id"
        class="live-img"
      />
      <!-- Offline placeholder -->
      <div v-else class="offline-placeholder">
        <div class="offline-label">NO SIGNAL</div>
        <div class="offline-sub">
          <template v-if="cam.offlineMin != null">LAST FRAME · {{ cam.offlineMin }}m AGO</template>
          <template v-else>INDEX {{ cam.index }}</template>
        </div>
      </div>

      <!-- Scanlines overlay -->
      <div v-if="cam.online && overlays" class="scanlines" style="position: absolute; inset: 0; pointer-events: none;" />

      <!-- Corner reticles -->
      <CameraReticle v-if="overlays" />

      <!-- Top-left HUD: live/offline pill + camera id -->
      <div v-if="overlays" class="hud hud-tl">
        <template v-if="cam.online">
          <span class="live-dot blink" />
          <span class="live-text">LIVE</span>
        </template>
        <template v-else>
          <span class="offline-dot" />
          <span class="offline-text">OFFLINE</span>
        </template>
        <span class="hud-sep">·</span>
        <span class="hud-id">{{ cam.id.toUpperCase() }}</span>
      </div>

      <!-- Top-right HUD: signal + resolution -->
      <div v-if="overlays && cam.online" class="hud hud-tr">
        <CameraSignalBars :strength="cam.signal" />
        <span>{{ cam.res ?? 'MJPEG' }}</span>
      </div>

      <!-- Bottom HUD: timestamp + fps/bitrate -->
      <div v-if="overlays && cam.online" class="hud-bottom">
        <div class="hud hud-bl">
          <span class="hud-key">TS</span>
          <span>{{ ts }}</span>
        </div>
        <div class="hud hud-br">
          <span class="hud-key">FPS</span>
          <span>{{ cam.targetFps ? String(fps).padStart(2, '0') : '—' }}</span>
          <span class="hud-sep">·</span>
          <span class="hud-key">BR</span>
          <span>{{ cam.bitrate != null ? `${cam.bitrate}k` : '—' }}</span>
        </div>
      </div>

      <!-- Crosshair on focused tile -->
      <svg
        v-if="focused"
        class="crosshair"
        width="48"
        height="48"
        viewBox="0 0 48 48"
        :style="{ color: accent }"
      >
        <circle cx="24" cy="24" r="14" fill="none" stroke="currentColor" stroke-width="1" opacity=".7"/>
        <line x1="24" y1="2"  x2="24" y2="14" stroke="currentColor" stroke-width="1"/>
        <line x1="24" y1="34" x2="24" y2="46" stroke="currentColor" stroke-width="1"/>
        <line x1="2"  y1="24" x2="14" y2="24" stroke="currentColor" stroke-width="1"/>
        <line x1="34" y1="24" x2="46" y2="24" stroke="currentColor" stroke-width="1"/>
      </svg>
    </div>

    <!-- Footer strip -->
    <div class="tile-footer">
      <span class="footer-label">{{ cam.id }}</span>
      <span class="footer-loc">INDEX {{ cam.index }}</span>
      <span class="footer-spacer" />
      <button
        class="snap-btn"
        :disabled="!cam.online"
        @click.stop="emit('snapshot', cam)"
      >SNAP ↗</button>
    </div>
  </div>
</template>

<style scoped>
.tile {
  position: relative;
  background: var(--panel);
  border: 1px solid var(--line);
  display: flex;
  flex-direction: column;
  cursor: pointer;
  min-height: 0;
  min-width: 0;
  transition: border-color .15s;
}
.tile--focused { border-color: var(--ink-dim); }

.feed-area {
  position: relative;
  flex: 1 1 auto;
  overflow: hidden;
  background: var(--bg-deep);
  min-height: 0;
}

.live-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.offline-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 6px;
  background: repeating-linear-gradient(135deg,
    oklch(0.13 0.005 240) 0 6px,
    oklch(0.155 0.005 240) 6px 12px);
}
.offline-label { font-size: 10px; color: var(--ink-faint); letter-spacing: .2em; }
.offline-sub { font-size: 9px; color: var(--ink-faint); }

/* HUD overlays */
.hud {
  position: absolute;
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 10px;
  color: var(--ink);
  background: oklch(0.10 0.005 240 / .55);
  padding: 3px 6px;
  border: 1px solid var(--line);
  letter-spacing: .04em;
}
.hud-tl { top: 10px; left: 10px; }
.hud-tr { top: 10px; right: 10px; color: var(--ink-dim); }
.hud-bottom {
  position: absolute;
  left: 10px;
  right: 10px;
  bottom: 10px;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 8px;
}
.hud-bl, .hud-br { position: static; }

.hud-key { color: var(--ink-faint); }
.hud-sep { color: var(--ink-faint); }
.live-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--live); flex-shrink: 0; }
.live-text { color: var(--live); font-weight: 600; letter-spacing: .12em; }
.offline-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent-2); flex-shrink: 0; }
.offline-text { color: var(--accent-2); font-weight: 600; letter-spacing: .12em; }
.hud-id { color: var(--ink-dim); }

.crosshair {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
}

/* footer strip */
.tile-footer {
  display: flex;
  border-top: 1px solid var(--line);
  font-size: 10px;
  color: var(--ink-dim);
  height: 26px;
  align-items: stretch;
  flex-shrink: 0;
}
.footer-label {
  padding: 6px 8px;
  border-right: 1px solid var(--line);
  color: var(--ink);
  letter-spacing: .06em;
  white-space: nowrap;
}
.footer-loc {
  padding: 6px 8px;
  border-right: 1px solid var(--line);
  color: var(--ink-faint);
  white-space: nowrap;
}
.footer-spacer { flex: 1; }
.snap-btn {
  border: none;
  border-left: 1px solid var(--line);
  background: transparent;
  color: var(--ink-dim);
  font: inherit;
  font-size: 10px;
  letter-spacing: .1em;
  padding: 0 10px;
  cursor: pointer;
  transition: background .12s, color .12s;
}
.snap-btn:hover:not(:disabled) { background: var(--panel-hi); color: var(--ink); }
.snap-btn:disabled { color: var(--ink-faint); cursor: not-allowed; }
</style>
