<script setup lang="ts">
defineProps<{ label: string; value: string; accent?: boolean }>()
</script>

<template>
  <div class="crumb">
    <span class="crumb-label">{{ label }}</span>
    <span :class="['crumb-value', { 'crumb-accent': accent }]">{{ value }}</span>
  </div>
</template>

<style scoped>
.crumb { display: flex; flex-direction: column; line-height: 1.1; }
.crumb-label { font-size: 9px; color: var(--ink-faint); letter-spacing: .15em; }
.crumb-value { font-size: 11px; color: var(--ink); letter-spacing: .04em; margin-top: 2px; }
.crumb-accent { color: var(--accent); }
</style>
