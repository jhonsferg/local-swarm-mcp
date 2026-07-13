<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { useLogStream } from "@/composables/useLogStream";

const { lines } = useLogStream();
const box = ref<HTMLElement | null>(null);

watch(lines, async () => {
  await nextTick();
  const el = box.value;
  if (!el) return;
  const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
  if (atBottom) el.scrollTop = el.scrollHeight;
});
</script>

<template>
  <div ref="box" class="log-viewer">
    <div v-for="(line, i) in lines" :key="i" class="log-viewer__line">{{ line }}</div>
    <div v-if="!lines.length" class="log-viewer__empty">waiting for log activity…</div>
  </div>
</template>

<style scoped lang="scss">
.log-viewer {
  @apply bg-black border border-border rounded-md p-3 h-80 overflow-y-auto font-mono text-xs whitespace-pre-wrap;

  &__line {
    @apply text-gray-300;
  }

  &__empty {
    @apply text-gray-600 italic;
  }
}
</style>
