export interface Backend {
  name: string;
  base_url: string;
  model: string;
}

export interface DiscoveredModel {
  name: string;
  capabilities?: string[];
}

export interface HostStatus {
  name: string;
  base_url: string;
  api_key?: string;
  models: DiscoveredModel[];
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
}
