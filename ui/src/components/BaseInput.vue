<script setup lang="ts">
withDefaults(defineProps<{ modelValue: string; placeholder?: string; type?: string; disabled?: boolean }>(), {
  disabled: false,
});
defineEmits<{ "update:modelValue": [string] }>();
</script>

<template>
  <input
    class="base-input"
    :class="{ 'base-input--disabled': disabled }"
    :type="type ?? 'text'"
    :placeholder="placeholder"
    :disabled="disabled"
    :value="modelValue"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>

<style scoped lang="scss">
.base-input {
  @apply bg-surface border border-border rounded px-2 py-1 text-sm text-gray-100 w-full;
  @apply focus:outline-none focus:ring-1 focus:ring-emerald-600;

  &--disabled {
    @apply opacity-50 cursor-not-allowed;
  }
}
</style>
