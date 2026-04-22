import React, { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

function StrategyModal({ initial, onSave, onClose }) {
  const [form, setForm] = useState(
    initial ?? { name: '', description: '', class_name: '', params: '{}' },
  );
  const [err, setErr] = useState('');

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  async function handleSubmit(e) {
    e.preventDefault();
    setErr('');
    let params;
    try {
      params = JSON.parse(form.params);
    } catch {
      setErr('params 不是合法 JSON');
      return;
    }
    try {
      await onSave({ name: form.name, description: form.description, class_name: form.class_name, params });
      onClose();
    } catch (e) {
      setErr(e.message);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold mb-4">{initial ? '编辑策略' : '新建策略'}</h2>
        <form onSubmit={handleSubmit} className="space-y-3">
          <label className="block text-sm font-medium">
            名称
            <input
              className="mt-1 block w-full border rounded px-3 py-1.5 text-sm"
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              required
            />
          </label>
          <label className="block text-sm font-medium">
            描述
            <input
              className="mt-1 block w-full border rounded px-3 py-1.5 text-sm"
              value={form.description}
              onChange={(e) => set('description', e.target.value)}
            />
          </label>
          <label className="block text-sm font-medium">
            类名 (class_name)
            <input
              className="mt-1 block w-full border rounded px-3 py-1.5 text-sm font-mono"
              value={form.class_name}
              onChange={(e) => set('class_name', e.target.value)}
              required
              placeholder="MACrossStrategy"
            />
          </label>
          <label className="block text-sm font-medium">
            参数 (JSON)
            <textarea
              className="mt-1 block w-full border rounded px-3 py-1.5 text-sm font-mono"
              rows={3}
              value={form.params}
              onChange={(e) => set('params', e.target.value)}
            />
          </label>
          {err && <p className="text-red-500 text-sm">{err}</p>}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-1.5 text-sm border rounded hover:bg-gray-50"
            >
              取消
            </button>
            <button
              type="submit"
              className="px-4 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
            >
              保存
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default function StrategiesPage() {
  const [strategies, setStrategies] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [modal, setModal] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.getStrategies();
      setStrategies(data.strategies ?? []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleSave(form) {
    if (modal.mode === 'create') {
      await api.createStrategy(form);
    } else {
      await api.updateStrategy(modal.strategy.id, form);
    }
    await load();
  }

  async function handleDelete(s) {
    if (!confirm(`确认删除策略「${s.name}」？`)) return;
    try {
      await api.deleteStrategy(s.id);
      await load();
    } catch (e) {
      alert(e.message);
    }
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-4">
        <h1 className="text-xl font-semibold">策略管理</h1>
        <button
          className="px-4 py-2 bg-blue-600 text-white text-sm rounded hover:bg-blue-700"
          onClick={() => setModal({ mode: 'create' })}
        >
          + 新建策略
        </button>
      </div>

      {error && <p className="text-red-500 mb-3">{error}</p>}

      {loading ? (
        <p className="text-gray-400 text-center py-8">加载中…</p>
      ) : strategies.length === 0 ? (
        <p className="text-gray-400 text-center py-8">暂无策略，点击右上角新建</p>
      ) : (
        <table className="w-full bg-white rounded-lg shadow text-sm">
          <thead className="bg-gray-50 text-gray-600">
            <tr>
              <th className="text-left px-4 py-3 font-medium">名称</th>
              <th className="text-left px-4 py-3 font-medium">类名</th>
              <th className="text-left px-4 py-3 font-medium">参数</th>
              <th className="text-left px-4 py-3 font-medium">创建时间</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y">
            {strategies.map((s) => (
              <tr key={s.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 font-medium">{s.name}</td>
                <td className="px-4 py-3 font-mono text-gray-600">{s.class_name}</td>
                <td className="px-4 py-3 font-mono text-xs text-gray-500">
                  {JSON.stringify(s.params)}
                </td>
                <td className="px-4 py-3 text-gray-400">{s.created_at?.slice(0, 10)}</td>
                <td className="px-4 py-3">
                  <div className="flex gap-2 justify-end">
                    <button
                      className="text-blue-600 hover:underline text-xs"
                      onClick={() =>
                        setModal({
                          mode: 'edit',
                          strategy: { ...s, params: JSON.stringify(s.params) },
                        })
                      }
                    >
                      编辑
                    </button>
                    <button
                      className="text-red-500 hover:underline text-xs"
                      onClick={() => handleDelete(s)}
                    >
                      删除
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {modal && (
        <StrategyModal
          initial={modal.strategy}
          onSave={handleSave}
          onClose={() => setModal(null)}
        />
      )}
    </div>
  );
}
