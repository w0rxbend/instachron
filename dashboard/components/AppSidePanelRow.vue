<script setup lang="ts">
import type { Camera } from '~/types/camera'

const props = defineProps<{ cam: Camera; focused: boolean }>()
defineEmits<{ click: [] }>()
</script>

<template>
  <div
    :class="['cam-row', { 'cam-row--focused': focused }]"
    @click="$emit('click')"
  >
    <span
      :class="['cam-status', { blink: cam.online }]"
      :style="{ background: cam.online ? 'var(--live)' : 'var(--accent-2)', opacity: cam.online ? 1 : 0.6 }"
    />
    <div class="cam-meta">
      <div class="cam-id-row">
        <span class="cam-id">{{ cam.id }}</span>
        <span class="cam-label">INDEX {{ cam.index }}</span>
      </div>
      <div class="cam-detail">
        <template v-if="cam.online">
          <span v-if="cam.targetFps || cam.res || cam.loc">
            {{ cam.targetFps ?? '—' }}fps · {{ cam.res ?? '—' }} · {{ cam.loc ?? '—' }}
          </span>
          <span v-else>id={{ cam.id }} · index={{ cam.index }}</span>
        </template>
        <template v-else>
          <span v-if="cam.offlineMin != null">last seen {{ cam.offlineMin }}m ago</span>
          <span v-else>offline</span>
        </template>
      </div>
    </div>
    <span class="cam-arrow">{{ cam.online ? '→' : '··' }}</span>
  </div>
</template>

<style scoped>
.cam-row {
  display: grid;
  grid-template-columns: 16px 1fr auto;
  gap: 10px;
  align-items: center;
  padding: 7px 14px;
  border-left: 2px solid transparent;
  cursor: pointer;
  transition: background .1s;
}
.cam-row:hover { background: oklch(0.16 0.008 240); }
.cam-row--focused {
  background: oklch(0.20 0.012 240);
  border-left-color: var(--accent);
}

.cam-status { width: 8px; height: 8px; display: block; }

.cam-meta { min-width: 0; }
.cam-id-row { font-size: 11px; color: var(--ink); display: flex; gap: 6px; align-items: baseline; }
.cam-id { letter-spacing: .04em; }
.cam-label { color: var(--ink-faint); font-size: 9px; }
.cam-detail {
  font-size: 9px;
  color: var(--ink-faint);
  letter-spacing: .05em;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cam-arrow { font-size: 9px; color: var(--ink-faint); }
</style>
