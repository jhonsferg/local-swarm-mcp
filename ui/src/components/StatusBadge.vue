<script setup lang="ts">
defineProps<{ online: boolean; onLabel?: string; offLabel?: string }>();
</script>

<template>
  <span class="status-badge" :class="online ? 'status-badge--online' : 'status-badge--offline'">
    <span class="status-badge__dot" />
    {{ online ? (onLabel ?? "online") : (offLabel ?? "offline") }}
  </span>
</template>

<style scoped>
@reference "../styles/main.css";

.status-badge {
  @apply inline-flex items-center gap-1.5 text-xs font-medium;
}
.status-badge__dot {
  @apply w-2 h-2 rounded-full inline-block;
}
.status-badge--online {
  @apply text-emerald-400;
}
.status-badge--online .status-badge__dot {
  @apply bg-emerald-400;
}
.status-badge--offline {
  @apply text-red-400;
}
.status-badge--offline .status-badge__dot {
  @apply bg-red-400;
}
</style>
