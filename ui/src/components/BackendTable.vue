<script setup lang="ts">
import type { Backend } from "@/types";

defineProps<{ backends: Backend[] }>();
</script>

<template>
  <table class="backend-table">
    <thead>
      <tr>
        <th class="backend-table__th">Name</th>
        <th class="backend-table__th">Model</th>
        <th class="backend-table__th">Base URL</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="b in backends" :key="b.name" class="backend-table__row">
        <td class="backend-table__td">{{ b.name }}</td>
        <td class="backend-table__td">{{ b.model }}</td>
        <td class="backend-table__td backend-table__td--muted">{{ b.base_url }}</td>
      </tr>
      <tr v-if="!backends.length">
        <td class="backend-table__td backend-table__td--muted" colspan="3">(no backends configured)</td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped lang="scss">
.backend-table {
  @apply w-full text-sm border-collapse;

  &__th {
    @apply text-left text-emerald-400 font-semibold border-b border-border pb-1;
  }

  &__row {
    @apply border-b border-border;
  }

  &__td {
    @apply py-1.5 pr-4;

    &--muted {
      @apply text-gray-400 italic;
    }
  }
}
</style>
