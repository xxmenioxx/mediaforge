import { request } from './client';

export type OllamaConnectionTestResponse = {
  state: string;
  checkedAt: string;
  serverVersion?: string;
  diagnostic?: string;
};

export type OllamaRuntimeModel = {
  modelId: string;
  installed: boolean;
  resident: boolean;
  state: string;
  activeInferenceLeases: number;
  expiresAt?: string;
  failure?: string;
};

export type OllamaRuntimeOperation = {
  operationId: string;
  modelId: string;
  phase: string;
  reason?: string;
};

export type OllamaRuntimeStatus = {
  state: string;
  observedAt?: string;
  diagnostic?: string;
  models: OllamaRuntimeModel[];
  activeOperations: OllamaRuntimeOperation[];
  failures: OllamaRuntimeOperation[];
};

export const testOllamaConnection = () => request<OllamaConnectionTestResponse>('/api/settings/ollama/test-connection', {
  method: 'POST',
  body: JSON.stringify({}),
});

export const ollamaRuntimeStatus = () => request<OllamaRuntimeStatus>('/api/settings/ollama/runtime');
