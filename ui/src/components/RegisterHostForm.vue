<script setup lang="ts">
import { ref, watch } from "vue";
import BaseButton from "./BaseButton.vue";
import BaseInput from "./BaseInput.vue";

const props = withDefaults(
  defineProps<{
    initialName?: string;
    initialBaseUrl?: string;
    initialApiKey?: string;
    lockName?: boolean; // true when editing an existing host - name is the key, don't let it change
    submitLabel?: string;
  }>(),
  { initialName: "", initialBaseUrl: "", initialApiKey: "", lockName: false, submitLabel: "Register" },
);

const emit = defineEmits<{
  submit: [{ name: string; baseUrl: string; apiKey: string }];
  cancel: [];
}>();

const name = ref(props.initialName);
const baseUrl = ref(props.initialBaseUrl);
const apiKey = ref(props.initialApiKey);

watch(
  () => [props.initialName, props.initialBaseUrl, props.initialApiKey],
  () => {
    name.value = props.initialName;
    baseUrl.value = props.initialBaseUrl;
    apiKey.value = props.initialApiKey;
  },
);

function onSubmit() {
  if (!name.value.trim() || !baseUrl.value.trim()) return;
  emit("submit", { name: name.value.trim(), baseUrl: baseUrl.value.trim(), apiKey: apiKey.value.trim() });
}
</script>

<template>
  <form class="register-host-form" @submit.prevent="onSubmit">
    <div class="register-host-form__field">
      <label class="register-host-form__label">Name</label>
      <BaseInput v-model="name" placeholder="rx9070" :disabled="lockName" />
    </div>
    <div class="register-host-form__field">
      <label class="register-host-form__label">Base URL (Ollama root, no /v1)</label>
      <BaseInput v-model="baseUrl" placeholder="http://192.168.18.29:11434" />
    </div>
    <div class="register-host-form__field">
      <label class="register-host-form__label">API key (optional)</label>
      <BaseInput v-model="apiKey" type="password" placeholder="" />
    </div>
    <div class="register-host-form__actions">
      <BaseButton type="submit">{{ submitLabel }}</BaseButton>
      <BaseButton variant="ghost" @click="$emit('cancel')">Cancel</BaseButton>
    </div>
  </form>
</template>

<style scoped lang="scss">
.register-host-form {
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
