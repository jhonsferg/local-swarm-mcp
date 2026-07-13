<script setup lang="ts">
import { ref } from "vue";
import { useAdminApi } from "@/composables/useAdminApi";
import { usePolling } from "@/composables/usePolling";
import BaseCard from "@/components/BaseCard.vue";
import BaseButton from "@/components/BaseButton.vue";
import BackendTable from "@/components/BackendTable.vue";
import HostCard from "@/components/HostCard.vue";
import McpServerCard from "@/components/McpServerCard.vue";
import RegisterHostForm from "@/components/RegisterHostForm.vue";
import RegisterMcpServerForm from "@/components/RegisterMcpServerForm.vue";
import LogViewer from "@/components/LogViewer.vue";

const api = useAdminApi();
const POLL_MS = 3000;

const { data: version } = usePolling(() => api.health().then((h) => h.version), 10000);
const { data: backends, refresh: refreshBackends } = usePolling(() => api.backends(), POLL_MS);
const { data: hosts, refresh: refreshHosts } = usePolling(() => api.hosts(), POLL_MS);
const { data: mcpServers, refresh: refreshMcpServers } = usePolling(() => api.mcpServers(), POLL_MS);

const showAddHost = ref(false);
const showAddMcpServer = ref(false);

async function onAddHost(v: { name: string; baseUrl: string; apiKey: string }) {
  await api.registerHost(v.name, v.baseUrl, v.apiKey);
  showAddHost.value = false;
  await refreshHosts();
}

async function onUpdateHost(v: { name: string; baseUrl: string; apiKey: string }) {
  await api.registerHost(v.name, v.baseUrl, v.apiKey);
  await refreshHosts();
}

async function onRemoveHost(name: string) {
  await api.unregisterHost(name);
  await refreshHosts();
}

async function onAddMcpServer(v: { name: string; command: string; args: string[] }) {
  await api.registerMCPServer(v.name, v.command, v.args);
  showAddMcpServer.value = false;
  await refreshMcpServers();
}

async function onRemoveMcpServer(name: string) {
  await api.unregisterMCPServer(name);
  await refreshMcpServers();
}
</script>

<template>
  <main class="app">
    <h1 class="app__title">
      🐝 local-swarm-mcp
      <span v-if="version" class="app__version">{{ version }}</span>
    </h1>

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

      <BaseButton v-if="!showAddMcpServer" variant="ghost" @click="showAddMcpServer = true">+ Register server</BaseButton>
      <RegisterMcpServerForm v-else @submit="onAddMcpServer" @cancel="showAddMcpServer = false" />
    </BaseCard>

    <BaseCard title="Live logs">
      <LogViewer />
    </BaseCard>
  </main>
</template>

<style scoped lang="scss">
.app {
  @apply max-w-4xl mx-auto p-6;

  &__title {
    @apply text-xl font-semibold mb-6;
  }

  &__version {
    @apply text-sm text-gray-500 font-normal ml-2;
  }

  &__empty {
    @apply text-gray-500 italic text-sm mb-2;
  }
}
</style>
