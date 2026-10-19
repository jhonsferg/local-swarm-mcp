<script setup lang="ts">
import type { MCPServerStatus } from "@/types";
import BaseButton from "./BaseButton.vue";
import StatusBadge from "./StatusBadge.vue";

defineProps<{ server: MCPServerStatus }>();
defineEmits<{ remove: [string] }>();
</script>

<template>
  <div class="mcp-server-card">
    <div class="mcp-server-card__header">
      <div>
        <span class="mcp-server-card__name">{{ server.name }}</span>
        <span class="mcp-server-card__command">{{ server.command }} {{ (server.args ?? []).join(" ") }}</span>
      </div>
      <StatusBadge :online="server.connected" on-label="connected" off-label="disconnected" />
    </div>
    <div class="mcp-server-card__actions">
      <BaseButton variant="danger" @click="$emit('remove', server.name)">Remove</BaseButton>
    </div>
  </div>
</template>

<style scoped>
@reference "../styles/main.css";

.mcp-server-card {
  @apply bg-panel border border-border rounded-md p-3 mb-2;
}
.mcp-server-card__header {
  @apply flex items-center justify-between;
}
.mcp-server-card__name {
  @apply font-semibold text-gray-100 mr-2;
}
.mcp-server-card__command {
  @apply text-xs text-gray-400;
}
.mcp-server-card__actions {
  @apply flex gap-2 mt-2;
}
</style>
