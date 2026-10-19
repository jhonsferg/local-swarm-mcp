<script setup lang="ts">
import { ref } from "vue";
import BaseButton from "./BaseButton.vue";
import BaseInput from "./BaseInput.vue";

defineProps<{ error?: string | null }>();
const emit = defineEmits<{ submit: [string] }>();

const token = ref("");

function onSubmit() {
  if (!token.value.trim()) return;
  emit("submit", token.value.trim());
  token.value = "";
}
</script>

<template>
  <form class="login-form" @submit.prevent="onSubmit">
    <p class="login-form__hint">This daemon requires its API key (the value of -api-key).</p>
    <BaseInput v-model="token" type="password" placeholder="API key" />
    <p v-if="error" class="login-form__error">{{ error }}</p>
    <div>
      <BaseButton type="submit">Sign in</BaseButton>
    </div>
  </form>
</template>

<style scoped>
@reference "../styles/main.css";

.login-form {
  @apply flex flex-col gap-2 bg-panel border border-border rounded-md p-3 max-w-sm;
}
.login-form__hint {
  @apply text-xs text-gray-400;
}
.login-form__error {
  @apply text-xs text-red-400;
}
</style>
