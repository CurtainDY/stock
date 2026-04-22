import { vi, beforeEach, afterEach, test, expect } from 'vitest';
import { api } from '../api/client';

beforeEach(() => {
  global.fetch = vi.fn();
});
afterEach(() => vi.restoreAllMocks());

test('getStrategies calls GET /v1/strategies', async () => {
  global.fetch.mockResolvedValueOnce({
    ok: true, status: 200,
    json: () => Promise.resolve({ strategies: [] }),
  });
  const result = await api.getStrategies();
  expect(global.fetch).toHaveBeenCalledWith(
    expect.stringContaining('/v1/strategies'),
    expect.objectContaining({ method: 'GET' }),
  );
  expect(result.strategies).toEqual([]);
});

test('createStrategy calls POST /v1/strategies with body', async () => {
  const payload = { name: 'Test', class_name: 'TestStrategy', params: {} };
  global.fetch.mockResolvedValueOnce({
    ok: true, status: 200,
    json: () => Promise.resolve({ id: 1, ...payload }),
  });
  const result = await api.createStrategy(payload);
  expect(global.fetch).toHaveBeenCalledWith(
    expect.stringContaining('/v1/strategies'),
    expect.objectContaining({ method: 'POST', body: JSON.stringify(payload) }),
  );
  expect(result.id).toBe(1);
});

test('deleteStrategy calls DELETE and returns null on 204', async () => {
  global.fetch.mockResolvedValueOnce({ ok: true, status: 204 });
  const result = await api.deleteStrategy(1);
  expect(global.fetch).toHaveBeenCalledWith(
    expect.stringContaining('/v1/strategies/1'),
    expect.objectContaining({ method: 'DELETE' }),
  );
  expect(result).toBeNull();
});

test('throws on non-ok response', async () => {
  global.fetch.mockResolvedValueOnce({
    ok: false, status: 500,
    text: () => Promise.resolve('Internal Server Error'),
  });
  await expect(api.getStrategies()).rejects.toThrow('500');
});
