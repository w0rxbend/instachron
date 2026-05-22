<script setup lang="ts">
import type { Camera } from '~/types/camera'

const props = defineProps<{
  cameras: Camera[]
  focusedId: string | null
}>()

const emit = defineEmits<{ focus: [id: string] }>()

const online = computed(() => props.cameras.filter(c => c.online))
const offline = computed(() => props.cameras.filter(c => !c.online))
</script>

<template>
  <aside class="side-panel">
    <!-- Header -->
    <div class="panel-header">
      <div>
        <div class="panel-title">CAMERA REGISTRY</div>
        <div class="panel-sub">{{ cameras.length }} NODES DISCOVERED</div>
      </div>
      <div class="header-actions">
        <button class="icon-btn">⊞</button>
        <button class="icon-btn">⌕</button>
      </div>
    </div>

    <!-- Poll indicator -->
    <div class="poll-row">
      <div class="poll-left">
        <span class="poll-dot blink" />
        <span>POLLING /cameras · 5s</span>
      </div>
      <span style="color: var(--ink-dim)">▸</span>
    </div>

    <!-- Camera lists -->
    <div class="cam-list">
      <!-- Online group -->
      <div class="cam-group-header">
        <span class="cam-group-dot" style="background: var(--live)" />
        <span>ONLINE</span>
        <span class="cam-group-rule" />
        <span class="cam-group-count">{{ String(online.length).padStart(2, '0') }}</span>
      </div>
      <AppSidePanelRow
        v-for="cam in online"
        :key="cam.id"
        :cam="cam"
        :focused="focusedId === cam.id"
        @click="emit('focus', cam.id)"
      />

      <!-- Offline group -->
      <div class="cam-group-header">
        <span class="cam-group-dot" style="background: var(--accent-2); opacity: .6" />
        <span>OFFLINE</span>
        <span class="cam-group-rule" />
        <span class="cam-group-count">{{ String(offline.length).padStart(2, '0') }}</span>
      </div>
      <AppSidePanelRow
        v-for="cam in offline"
        :key="cam.id"
        :cam="cam"
        :focused="focusedId === cam.id"
        @click="emit('focus', cam.id)"
      />
    </div>

    <!-- Keybind footer -->
    <div class="kbd-footer">
      <span><span class="kbd">G</span> grid</span>
      <span><span class="kbd">F</span> focus</span>
      <span><span class="kbd">S</span> snap</span>
      <span><span class="kbd">␣</span> pause</span>
    </div>
  </aside>
</template>

<style scoped>
.side-panel {
  width: 260px;
  flex: 0 0 260px;
  border-right: 1px solid var(--line);
  background: oklch(0.125 0.006 240 / .9);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.panel-header {
  padding: 12px 14px;
  border-bottom: 1px solid var(--line);
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
}
.panel-title { font-size: 11px; color: var(--ink); letter-spacing: .08em; font-weight: 600; }
.panel-sub { font-size: 9px; color: var(--ink-faint); letter-spacing: .12em; margin-top: 3px; }
.header-actions { display: flex; gap: 4px; }

.icon-btn {
  width: 22px;
  height: 22px;
  border: 1px solid var(--line);
  background: transparent;
  color: var(--ink-dim);
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
  transition: background .12s, color .12s;
}
.icon-btn:hover { background: var(--panel-hi); color: var(--ink); }

.poll-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  border-bottom: 1px solid var(--line);
  font-size: 10px;
  color: var(--ink-faint);
  letter-spacing: .1em;
}
.poll-left { display: flex; align-items: center; gap: 8px; }
.poll-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--accent); }

.cam-list { flex: 1; overflow-y: auto; }

.cam-group-header {
  padding: 10px 14px 6px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 9px;
  color: var(--ink-faint);
  letter-spacing: .18em;
}
.cam-group-dot { width: 5px; height: 5px; border-radius: 50%; flex-shrink: 0; }
.cam-group-rule { flex: 1; height: 1px; background: var(--line); }
.cam-group-count { color: var(--ink-dim); }

.kbd-footer {
  border-top: 1px solid var(--line);
  padding: 10px 14px;
  font-size: 10px;
  color: var(--ink-faint);
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
}
</style>
