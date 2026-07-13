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
      :initial-api-key="host.api_key ?? ''"
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

<style scoped lang="scss">
.host-card {
  @apply bg-panel border border-border rounded-md p-3 mb-2;

  &__header {
    @apply flex items-center justify-between;
  }

  &__name {
    @apply font-semibold text-gray-100 mr-2;
  }

  &__url {
    @apply text-xs text-gray-400;
  }

  &__models {
    @apply mt-2 flex flex-wrap gap-2 text-xs;
  }

  &__model {
    @apply bg-surface border border-border rounded px-2 py-0.5 text-gray-300;
  }

  &__caps {
    @apply text-gray-500;
  }

  &__empty {
    @apply text-gray-500 italic;
  }

  &__error {
    @apply mt-2 text-xs text-red-400;
  }

  &__actions {
    @apply flex gap-2 mt-2;
  }
}
</style>
