<script setup lang="ts">
defineProps<{ k: string; v?: string; mono?: boolean }>()
</script>

<template>
  <div class="kv">
    <span class="kv-key">{{ k }}</span>
    <span class="kv-val">
      <slot>{{ v }}</slot>
    </span>
  </div>
</template>

<style scoped>
.kv { display: grid; grid-template-columns: 110px 1fr; gap: 8px; font-size: 11px; }
.kv-key { color: var(--ink-faint); letter-spacing: .08em; }
.kv-val { color: var(--ink); word-break: break-all; }
</style>
