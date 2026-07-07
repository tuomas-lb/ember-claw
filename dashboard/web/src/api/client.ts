const API_BASE = '/api';

export interface Instance {
  name: string;
  status: string;
  model: string;
  provider: string;
  ready: boolean;
  age: string;
  cpu_limit: string;
  memory_limit: string;
}

export interface InstanceStatus extends Instance {
  grpc_status?: {
    ready: boolean;
    model: string;
    provider: string;
    uptime_seconds: number;
  };
  pod_name: string;
  pod_ip: string;
}

export interface DeployRequest {
  name: string;
  provider: string;
  model: string;
  api_key: string;
  cpu_limit?: string;
  memory_limit?: string;
  storage_size?: string;
}

export interface ChatStep {
  kind: 'reasoning' | 'tool';
  tool?: string;
  content?: string;
}

export interface ChatMessage {
  text: string;
  done: boolean;
  error?: string;
  // Present on intermediate frames while the agent is still working.
  step?: ChatStep;
}

export interface Provider {
  name: string;
  api_base: string;
}

async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options?.headers },
    ...options,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`API ${res.status}: ${text}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function listInstances(): Promise<Instance[]> {
  return apiFetch<Instance[]>('/instances');
}

export async function getInstance(name: string): Promise<InstanceStatus> {
  return apiFetch<InstanceStatus>(`/instances/${name}`);
}

export async function deployInstance(req: DeployRequest): Promise<void> {
  return apiFetch<void>('/instances', {
    method: 'POST',
    body: JSON.stringify(req),
  });
}

export async function deleteInstance(name: string): Promise<void> {
  return apiFetch<void>(`/instances/${name}`, { method: 'DELETE' });
}

export async function restartInstance(name: string): Promise<void> {
  return apiFetch<void>(`/instances/${name}/restart`, { method: 'POST' });
}

export async function getConfig(name: string): Promise<unknown> {
  return apiFetch<unknown>(`/instances/${name}/config`);
}

export async function pushConfig(name: string, config: unknown): Promise<void> {
  return apiFetch<void>(`/instances/${name}/config`, {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

export async function listProviders(): Promise<Provider[]> {
  return apiFetch<Provider[]>('/providers');
}

// WebSocket base URL derived from current page location
function wsBase(): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${window.location.host}`;
}

export function connectLogs(
  name: string,
  onMessage: (line: string) => void,
  follow = true,
): WebSocket {
  const url = `${wsBase()}/api/instances/${name}/logs?follow=${follow}&tail=200`;
  const ws = new WebSocket(url);
  ws.onmessage = (e) => onMessage(e.data as string);
  return ws;
}

export function connectChat(name: string): WebSocket {
  const url = `${wsBase()}/api/instances/${name}/chat`;
  return new WebSocket(url);
}

// Persisted chat history
export interface StoredMessage {
  id: number;
  instance: string;
  session_id: string;
  role: 'user' | 'agent';
  content: string;
  created_at: string;
}

export async function fetchMessages(name: string, session?: string, limit = 500): Promise<StoredMessage[]> {
  const params = new URLSearchParams();
  if (session) params.set('session', session);
  params.set('limit', String(limit));
  return apiFetch<StoredMessage[]>(`/instances/${name}/messages?${params.toString()}`);
}

// Models
export interface ProviderModel {
  id: string;
  display_name: string;
}

export async function listModels(provider: string, apiKey: string): Promise<ProviderModel[]> {
  const res = await fetch(`${API_BASE}/providers/${provider}/models?api_key=${encodeURIComponent(apiKey)}`);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

// Call routing
export async function getCallRouting(): Promise<Record<string, string>> {
  const res = await fetch(`${API_BASE}/telephony/routing`);
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function putCallRouting(data: Record<string, string>): Promise<void> {
  const res = await fetch(`${API_BASE}/telephony/routing`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(await res.text());
}

// Fleet.md
export async function getFleetMD(): Promise<string> {
  const res = await fetch(`${API_BASE}/fleet`);
  if (!res.ok) throw new Error(await res.text());
  const data = await res.json();
  return data.content || '';
}

export async function putFleetMD(content: string): Promise<void> {
  const res = await fetch(`${API_BASE}/fleet`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) throw new Error(await res.text());
}
