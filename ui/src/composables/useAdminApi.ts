import type { Backend, HealthResponse, HostStatus, MCPServerStatus } from "@/types";

/** Thrown for a 401: the daemon wants the API key (see login). */
export class AuthError extends Error {
  constructor() {
    super("authentication required");
    this.name = "AuthError";
  }
}

// Thin wrapper around the daemon's /admin/* HTTP surface. Kept as plain
// fetch calls (no client-generated SDK) since the surface is small and
// stable - see internal/admin in the Go server for the source of truth.
// Authentication rides on the HttpOnly session cookie POST /admin/login
// sets, so no token is ever held in page script.
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin", ...init });
  if (res.status === 401) throw new AuthError();
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(`${init?.method ?? "GET"} ${path} failed: ${res.status} ${body}`.trim());
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function postJSON<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function useAdminApi() {
  return {
    login: (token: string) => postJSON<void>("/admin/login", { token }),
    logout: () => postJSON<void>("/admin/logout", {}),
    health: () => request<HealthResponse>("/admin/health"),
    backends: () => request<Backend[]>("/admin/backends"),
    hosts: () => request<HostStatus[]>("/admin/hosts"),
    // An empty apiKey keeps the host's stored key (the daemon never sends
    // it back, so an edit form can't pre-fill it).
    registerHost: (name: string, baseUrl: string, apiKey?: string) =>
      postJSON<HostStatus>("/admin/register-host", { name, base_url: baseUrl, api_key: apiKey || undefined }),
    unregisterHost: (name: string) => postJSON<void>("/admin/unregister-host", { name }),
    mcpServers: () => request<MCPServerStatus[]>("/admin/mcp-servers"),
    registerMCPServer: (name: string, command: string, args: string[]) =>
      postJSON<MCPServerStatus>("/admin/register-mcp-server", { name, command, args: args.length ? args : undefined }),
    unregisterMCPServer: (name: string) => postJSON<void>("/admin/unregister-mcp-server", { name }),
  };
}

export type AdminApi = ReturnType<typeof useAdminApi>;
