<script setup lang="ts">
import { ref } from "vue";
import type { HostStatus } from "@/types";
import BaseButton from "./BaseButton.vue";
import StatusBadge from "./StatusBadge.vue";
import RegisterHostForm from "./RegisterHostForm.vue";

defineProps<{ host: HostStatus }>();
const emit = defineEmits<{
  update: [{ name: string; baseUrl: string; apiKey: string }];
  remove: [string];
}>();

const editing = ref(false);
</script>

<template>
  <div class="host-card">
    <div class="host-card__header">
      <div>
        <span class="host-card__name">{{ host.name }}</span>
        <span class="host-card__url">{{ host.base_url }}</span>
        <span v-if="host.has_api_key" class="host-card__key">key set</span>
      </div>
      <StatusBadge :online="host.up" />
    </div>

    <div class="host-card__models">
      <span v-if="!host.models?.length" class="host-card__empty">no models discovered yet</span>
      <span v-for="m in host.models" :key="m.name" class="host-card__model">
        {{ m.name }}
        <span v-if="m.capabilities?.length" class="host-card__caps">({{ m.capabilities.join(", ") }})</span>
      </span>
    </div>

    <div v-if="host.last_error" class="host-card__error">{{ host.last_error }}</div>

    <div class="host-card__actions">
      <BaseButton variant="ghost" @click="editing = !editing">{{ editing ? "Close" : "Edit" }}</BaseButton>
      <BaseButton variant="danger" @click="emit('remove', host.name)">Remove</BaseButton>
    </div>

    <RegisterHostForm
      v-if="editing"
      :initial-name="host.name"
      :initial-base-url="host.base_url"
      :api-key-placeholder="host.has_api_key ? 'leave blank to keep the current key' : ''"
      lock-name
      submit-label="Save"
      @submit="
        (v) => {
          emit('update', v);
          editing = false;
        }
      "
      @cancel="editing = false"
    />
  </div>
</template>

<style scoped>
@reference "../styles/main.css";

.host-card {
  @apply bg-panel border border-border rounded-md p-3 mb-2;
}
.host-card__header {
  @apply flex items-center justify-between;
}
.host-card__name {
  @apply font-semibold text-gray-100 mr-2;
}
.host-card__url {
  @apply text-xs text-gray-400;
}
.host-card__key {
  @apply text-xs text-gray-500 ml-2;
}
.host-card__models {
  @apply mt-2 flex flex-wrap gap-2 text-xs;
}
.host-card__model {
  @apply bg-surface border border-border rounded px-2 py-0.5 text-gray-300;
}
.host-card__caps {
  @apply text-gray-500;
}
.host-card__empty {
  @apply text-gray-500 italic;
}
.host-card__error {
  @apply mt-2 text-xs text-red-400;
}
.host-card__actions {
  @apply flex gap-2 mt-2;
}
</style>
