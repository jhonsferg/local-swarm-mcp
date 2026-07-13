import type { Backend, HealthResponse, HostStatus, MCPServerStatus } from "@/types";

// Thin wrapper around the daemon's /admin/* HTTP surface. Kept as plain
// fetch calls (no client-generated SDK) since the surface is small and
// stable - see internal/admin in the Go server for the source of truth.
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${init?.method ?? "GET"} ${path} failed: ${res.status} ${body}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function useAdminApi() {
  return {
    health: () => request<HealthResponse>("/admin/health"),
    backends: () => request<Backend[]>("/admin/backends"),
    hosts: () => request<HostStatus[]>("/admin/hosts"),
    registerHost: (name: string, baseUrl: string, apiKey?: string) =>
      request<HostStatus>("/admin/register-host", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, base_url: baseUrl, api_key: apiKey || undefined }),
      }),
    unregisterHost: (name: string) =>
      request<void>("/admin/unregister-host", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      }),
    mcpServers: () => request<MCPServerStatus[]>("/admin/mcp-servers"),
    registerMCPServer: (name: string, command: string, args: string[]) =>
      request<MCPServerStatus>("/admin/register-mcp-server", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, command, args: args.length ? args : undefined }),
      }),
    unregisterMCPServer: (name: string) =>
      request<void>("/admin/unregister-mcp-server", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      }),
  };
}

export type AdminApi = ReturnType<typeof useAdminApi>;
