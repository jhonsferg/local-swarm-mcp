<script setup lang="ts">
import { ref } from "vue";
import BaseButton from "./BaseButton.vue";
import BaseInput from "./BaseInput.vue";

const emit = defineEmits<{
  submit: [{ name: string; command: string; args: string[] }];
  cancel: [];
}>();

const name = ref("");
const command = ref("");
const argsRaw = ref("");

function onSubmit() {
  if (!name.value.trim() || !command.value.trim()) return;
  const args = argsRaw.value
    .split(" ")
    .map((a) => a.trim())
    .filter(Boolean);
  emit("submit", { name: name.value.trim(), command: command.value.trim(), args });
  name.value = "";
  command.value = "";
  argsRaw.value = "";
}
</script>

<template>
  <form class="register-mcp-form" @submit.prevent="onSubmit">
    <div class="register-mcp-form__field">
      <label class="register-mcp-form__label">Name</label>
      <BaseInput v-model="name" placeholder="codebase-memory-mcp" />
    </div>
    <div class="register-mcp-form__field">
      <label class="register-mcp-form__label">Command</label>
      <BaseInput v-model="command" placeholder="C:/tools/codebase-memory-mcp.exe" />
    </div>
    <div class="register-mcp-form__field">
      <label class="register-mcp-form__label">Args (space-separated, optional)</label>
      <BaseInput v-model="argsRaw" placeholder="" />
    </div>
    <div class="register-mcp-form__actions">
      <BaseButton type="submit">Register</BaseButton>
      <BaseButton variant="ghost" @click="$emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>

<style scoped lang="scss">
.register-mcp-form {
  @apply flex flex-col gap-2 bg-panel border border-border rounded-md p-3 mt-2;

  &__field {
    @apply flex flex-col gap-1;
  }

  &__label {
    @apply text-xs text-gray-400;
  }

  &__actions {
    @apply flex gap-2 mt-1;
  }
}
</style>
