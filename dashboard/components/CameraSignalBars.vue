<script setup lang="ts">
defineProps<{ strength?: number }>()
</script>

<template>
  <div class="signal">
    <div
      v-for="i in 4"
      :key="i"
      class="bar"
      :style="{
        height: `${3 + i * 1.8}px`,
        background: i <= (strength ?? 4) ? 'var(--ink-dim)' : 'var(--line)',
      }"
    />
  </div>
</template>

<style scoped>
.signal { display: inline-flex; gap: 2px; align-items: flex-end; height: 10px; }
.bar { width: 2px; }
</style>
