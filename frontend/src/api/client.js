const BASE = import.meta.env.VITE_API_BASE ?? '';

async function request(method, path, body) {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: body != null ? { 'Content-Type': 'application/json' } : {},
    body: body != null ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${method} ${path} → ${res.status}: ${text}`);
  }
  if (res.status === 204) return null;
  return res.json();
}

export const api = {
  getStrategies: () => request('GET', '/v1/strategies'),
  createStrategy: (data) => request('POST', '/v1/strategies', data),
  updateStrategy: (id, data) => request('PUT', `/v1/strategies/${id}`, data),
  deleteStrategy: (id) => request('DELETE', `/v1/strategies/${id}`),
  getSymbols: (freq = '1m') => request('GET', `/v1/symbols?freq=${encodeURIComponent(freq)}`),
  runBacktest: (data) => request('POST', '/v1/backtests', data),
  getBacktests: () => request('GET', '/v1/backtests'),
  getBacktest: (id) => request('GET', `/v1/backtests/${id}`),
};
