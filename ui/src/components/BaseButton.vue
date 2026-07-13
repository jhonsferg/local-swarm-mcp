<script setup lang="ts">
withDefaults(
  defineProps<{
    variant?: "primary" | "danger" | "ghost";
    type?: "button" | "submit";
    disabled?: boolean;
  }>(),
  { variant: "primary", type: "button", disabled: false },
);

defineEmits<{ click: [MouseEvent] }>();
</script>

<template>
  <button
    :type="type"
    :disabled="disabled"
    class="base-btn"
    :class="[`base-btn--${variant}`, { 'base-btn--disabled': disabled }]"
    @click="$emit('click', $event)"
  >
    <slot />
  </button>
</template>

<style scoped lang="scss">
.base-btn {
  @apply rounded-md px-3 py-1.5 text-sm font-medium transition-colors;

  &--primary {
    @apply bg-emerald-600 text-white hover:bg-emerald-500;
  }

  &--danger {
    @apply bg-red-700 text-white hover:bg-red-600;
  }

  &--ghost {
    @apply bg-transparent text-gray-300 border border-border hover:bg-panel;
  }

  &--disabled {
    @apply opacity-50 cursor-not-allowed pointer-events-none;
  }
}
</style>
