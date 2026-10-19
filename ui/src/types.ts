export interface Backend {
  name: string;
  base_url: string;
  model: string;
  has_api_key?: boolean;
}

export interface DiscoveredModel {
  name: string;
  capabilities?: string[];
}

// API keys are never sent to the dashboard; has_api_key only says whether
// one is stored.
export interface HostStatus {
  name: string;
  base_url: string;
  has_api_key?: boolean;
  /** @deprecated never sent by the daemon any more; kept until HostCard stops reading it. */
  api_key?: string;
  models: DiscoveredModel[] | null;
  up: boolean;
  last_seen: string;
  last_error?: string;
}

export interface MCPServerStatus {
  name: string;
  command: string;
  args?: string[];
  connected: boolean;
}

export interface HealthResponse {
  service: string;
  version: string;
  command_registration?: boolean;
  auth_required?: boolean;
}
