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

<style scoped lang="scss">
.base-card {
  @apply mb-7;

  &__title {
    @apply text-xs uppercase tracking-wide text-gray-400 border-b border-border pb-1 mb-3;
  }

  &__body {
    @apply text-sm;
  }
}
</style>
