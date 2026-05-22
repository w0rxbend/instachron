<script setup lang="ts">
import type { EventEntry } from '~/types/camera'

defineProps<{ events: EventEntry[] }>()

const activeTab = ref<'ALL' | 'MOTION' | 'SYSTEM' | 'NET'>('ALL')
const tabs = ['ALL', 'MOTION', 'SYSTEM', 'NET'] as const

function lvlColor(lvl: string) {
  switch (lvl) {
    case 'WARN': return 'var(--warn)'
    case 'ERR':  return 'var(--accent-2)'
    case 'OK':   return 'var(--live)'
    default:     return 'var(--ink-faint)'
  }
}
</script>

<template>
  <div class="event-log">
    <!-- Header row -->
    <div class="log-header">
      <span class="log-title">EVENT LOG</span>
      <span class="log-sep">·</span>
      <span>STREAM ◉</span>
      <span class="log-spacer" />
      <button
        v-for="tab in tabs"
        :key="tab"
        :class="['log-tab', { 'log-tab--active': activeTab === tab }]"
        @click="activeTab = tab"
      >{{ tab }}</button>
    </div>

    <!-- Entries -->
    <div class="log-body">
      <div v-for="(e, i) in events" :key="i" class="log-row">
        <span class="log-time">{{ e.t }}</span>
        <span class="log-lvl" :style="{ color: lvlColor(e.lvl) }">{{ e.lvl }}</span>
        <span class="log-src">{{ e.src }}</span>
        <span class="log-msg">{{ e.msg }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.event-log {
  border-top: 1px solid var(--line);
  background: oklch(0.115 0.006 240);
  height: 128px;
  display: flex;
  flex-direction: column;
  flex: 0 0 128px;
}

.log-header {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 6px 14px;
  border-bottom: 1px solid var(--line);
  font-size: 10px;
  letter-spacing: .15em;
  color: var(--ink-faint);
  flex-shrink: 0;
}
.log-title { color: var(--ink); }
.log-sep { color: var(--ink-faint); }
.log-spacer { flex: 1; }

.log-tab {
  font: inherit;
  font-size: 10px;
  letter-spacing: .12em;
  color: var(--ink-faint);
  background: transparent;
  border: none;
  border-bottom: 1px solid transparent;
  padding: 2px 0;
  cursor: pointer;
  transition: color .12s;
}
.log-tab--active { color: var(--ink); border-bottom-color: var(--accent); }
.log-tab:hover:not(.log-tab--active) { color: var(--ink-dim); }

.log-body {
  flex: 1;
  overflow-y: auto;
  padding: 4px 0;
  font-size: 11px;
}

.log-row {
  display: grid;
  grid-template-columns: 96px 70px 72px 1fr;
  gap: 14px;
  padding: 3px 14px;
  color: var(--ink-dim);
}
.log-time { color: var(--ink-faint); }
.log-src { color: var(--accent); }
.log-msg { color: var(--ink); }
</style>
