<script setup lang="ts">
const props = defineProps<{
  label: string
  value: string
  dot?: 'live' | 'red' | 'warn' | 'accent'
}>()

const dotColor = computed(() => {
  switch (props.dot) {
    case 'live': return 'var(--live)'
    case 'red': return 'var(--accent-2)'
    case 'warn': return 'var(--warn)'
    case 'accent': return 'var(--accent)'
    default: return null
  }
})
</script>

<template>
  <div class="chip">
    <div class="chip-label">{{ label }}</div>
    <div class="chip-value">
      <span v-if="dotColor" class="chip-dot" :style="{ background: dotColor }" />
      {{ value }}
    </div>
  </div>
</template>

<style scoped>
.chip {
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 0 14px;
  border-left: 1px solid var(--line);
  min-width: 78px;
}
.chip-label { font-size: 9px; color: var(--ink-faint); letter-spacing: .15em; }
.chip-value {
  font-size: 11px;
  color: var(--ink);
  margin-top: 2px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.chip-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
</style>
