<script setup lang="ts">
defineProps<{ color?: string }>()
const ARM = 14
</script>

<template>
  <svg :style="{ position: 'absolute', top: '6px', left: '6px', color: color ?? 'var(--ink-dim)', pointerEvents: 'none' }" width="22" height="22" viewBox="0 0 22 22">
    <path :d="`M0 ${ARM} L0 0 L${ARM} 0`" stroke="currentColor" :stroke-width="1.5" fill="none"/>
  </svg>
  <svg :style="{ position: 'absolute', top: '6px', right: '6px', color: color ?? 'var(--ink-dim)', pointerEvents: 'none' }" width="22" height="22" viewBox="0 0 22 22">
    <path :d="`M${22 - ARM} 0 L22 0 L22 ${ARM}`" stroke="currentColor" :stroke-width="1.5" fill="none"/>
  </svg>
  <svg :style="{ position: 'absolute', bottom: '6px', left: '6px', color: color ?? 'var(--ink-dim)', pointerEvents: 'none' }" width="22" height="22" viewBox="0 0 22 22">
    <path :d="`M0 ${22 - ARM} L0 22 L${ARM} 22`" stroke="currentColor" :stroke-width="1.5" fill="none"/>
  </svg>
  <svg :style="{ position: 'absolute', bottom: '6px', right: '6px', color: color ?? 'var(--ink-dim)', pointerEvents: 'none' }" width="22" height="22" viewBox="0 0 22 22">
    <path :d="`M${22 - ARM} 22 L22 22 L22 ${22 - ARM}`" stroke="currentColor" :stroke-width="1.5" fill="none"/>
  </svg>
</template>
