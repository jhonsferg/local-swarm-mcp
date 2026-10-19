<script setup lang="ts">
import { ref } from "vue";
import { AuthError, useAdminApi } from "@/composables/useAdminApi";
import { usePolling } from "@/composables/usePolling";
import BaseCard from "@/components/BaseCard.vue";
import BaseButton from "@/components/BaseButton.vue";
import BackendTable from "@/components/BackendTable.vue";
import HostCard from "@/components/HostCard.vue";
import LoginForm from "@/components/LoginForm.vue";
import McpServerCard from "@/components/McpServerCard.vue";
import RegisterHostForm from "@/components/RegisterHostForm.vue";
import RegisterMcpServerForm from "@/components/RegisterMcpServerForm.vue";
import LogViewer from "@/components/LogViewer.vue";

const api = useAdminApi();
const POLL_MS = 3000;

// needsLogin flips on whenever any request comes back 401, and off after a
// successful sign-in; the session itself is an HttpOnly cookie.
const needsLogin = ref(false);
const loginError = ref<string | null>(null);
const actionError = ref<string | null>(null);

function guarded<T>(fetcher: () => Promise<T>): () => Promise<T> {
  return async () => {
    try {
      return await fetcher();
    } catch (e) {
      if (e instanceof AuthError) needsLogin.value = true;
      throw e;
    }
  };
}

const { data: health, refresh: refreshHealth } = usePolling(guarded(() => api.health()), 10000);
const { data: backends, refresh: refreshBackends } = usePolling(guarded(() => api.backends()), POLL_MS);
const { data: hosts, refresh: refreshHosts } = usePolling(guarded(() => api.hosts()), POLL_MS);
const { data: mcpServers, refresh: refreshMcpServers } = usePolling(guarded(() => api.mcpServers()), POLL_MS);

const showAddHost = ref(false);
const showAddMcpServer = ref(false);

async function refreshAll() {
  await Promise.all([refreshHealth(), refreshBackends(), refreshHosts(), refreshMcpServers()]);
}

async function onLogin(token: string) {
  try {
    await api.login(token);
    loginError.value = null;
    needsLogin.value = false;
    await refreshAll();
  } catch (e) {
    loginError.value = e instanceof AuthError ? "That key was not accepted." : String(e instanceof Error ? e.message : e);
  }
}

async function onLogout() {
  await api.logout().catch(() => undefined);
  needsLogin.value = true;
}

// run performs a user action, surfacing any failure in the banner instead
// of letting it vanish as an unhandled promise rejection.
async function run(action: () => Promise<unknown>, after?: () => Promise<void>) {
  actionError.value = null;
  try {
    await action();
  } catch (e) {
    if (e instanceof AuthError) {
      needsLogin.value = true;
      return;
    }
    actionError.value = e instanceof Error ? e.message : String(e);
  }
  await after?.();
}

function onAddHost(v: { name: string; baseUrl: string; apiKey: string }) {
  return run(
    () => api.registerHost(v.name, v.baseUrl, v.apiKey),
    async () => {
      showAddHost.value = false;
      await Promise.all([refreshHosts(), refreshBackends()]);
    },
  );
}

function onUpdateHost(v: { name: string; baseUrl: string; apiKey: string }) {
  return run(() => api.registerHost(v.name, v.baseUrl, v.apiKey), () => Promise.all([refreshHosts(), refreshBackends()]).then(() => undefined));
}

function onRemoveHost(name: string) {
  return run(() => api.unregisterHost(name), () => Promise.all([refreshHosts(), refreshBackends()]).then(() => undefined));
}

function onAddMcpServer(v: { name: string; command: string; args: string[] }) {
  return run(
    () => api.registerMCPServer(v.name, v.command, v.args),
    async () => {
      showAddMcpServer.value = false;
      await refreshMcpServers();
    },
  );
}

function onRemoveMcpServer(name: string) {
  return run(() => api.unregisterMCPServer(name), refreshMcpServers);
}
</script>

<template>
  <main class="app">
    <h1 class="app__title">
      🐝 local-swarm-mcp
      <span v-if="health" class="app__version">{{ health.version }}</span>
      <BaseButton v-if="!needsLogin && health?.auth_required" variant="ghost" class="app__logout" @click="onLogout">Sign out</BaseButton>
    </h1>

    <BaseCard v-if="needsLogin" title="Sign in">
      <LoginForm :error="loginError" @submit="onLogin" />
    </BaseCard>

    <template v-else>
      <div v-if="actionError" class="app__error" role="alert">
        {{ actionError }}
        <button class="app__error-close" aria-label="Dismiss" @click="actionError = null">×</button>
      </div>

      <BaseCard title="Backends">
        <BackendTable :backends="backends ?? []" />
      </BaseCard>

      <BaseCard title="Inference hosts">
        <HostCard
          v-for="h in hosts ?? []"
          :key="h.name"
          :host="h"
          @update="onUpdateHost"
          @remove="onRemoveHost"
        />
        <p v-if="hosts && !hosts.length" class="app__empty">No hosts registered yet.</p>

        <BaseButton v-if="!showAddHost" variant="ghost" @click="showAddHost = true">+ Register host</BaseButton>
        <RegisterHostForm v-else @submit="onAddHost" @cancel="showAddHost = false" />
      </BaseCard>

      <BaseCard title="Downstream MCP servers">
        <McpServerCard v-for="s in mcpServers ?? []" :key="s.name" :server="s" @remove="onRemoveMcpServer" />
        <p v-if="mcpServers && !mcpServers.length" class="app__empty">No downstream MCP servers registered yet.</p>

        <template v-if="health?.command_registration">
          <BaseButton v-if="!showAddMcpServer" variant="ghost" @click="showAddMcpServer = true">+ Register server</BaseButton>
          <RegisterMcpServerForm v-else @submit="onAddMcpServer" @cancel="showAddMcpServer = false" />
        </template>
        <p v-else-if="health" class="app__empty">
          Registering new servers is disabled on this daemon (start it with -allow-command-registration to enable).
        </p>
      </BaseCard>

      <BaseCard title="Live logs">
        <LogViewer />
      </BaseCard>
    </template>
  </main>
</template>

<style scoped>
@reference "./styles/main.css";

.app {
  @apply max-w-4xl mx-auto p-6;
}
.app__title {
  @apply text-xl font-semibold mb-6;
}
.app__version {
  @apply text-sm text-gray-500 font-normal ml-2;
}
.app__empty {
  @apply text-gray-500 italic text-sm mb-2;
}
.app__logout {
  @apply ml-3 align-middle;
}
.app__error {
  @apply flex items-start justify-between gap-3 bg-red-950 border border-red-800 text-red-200 text-sm rounded-md p-3 mb-6;
}
.app__error-close {
  @apply text-red-300 hover:text-white;
}
</style>
