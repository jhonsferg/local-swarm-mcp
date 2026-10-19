<script setup lang="ts">
defineProps<{ title: string }>();
</script>

<template>
  <section class="base-card">
    <h2 class="base-card__title">{{ title }}</h2>
    <div class="base-card__body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
@reference "../styles/main.css";

.base-card {
  @apply mb-7;
}
.base-card__title {
  @apply text-xs uppercase tracking-wide text-gray-400 border-b border-border pb-1 mb-3;
}
.base-card__body {
  @apply text-sm;
}
</style>
