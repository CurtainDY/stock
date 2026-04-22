import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';

export default function BacktestPage() {
  const navigate = useNavigate();
  const [strategies, setStrategies] = useState([]);
  const [runs, setRuns] = useState([]);
  const [form, setForm] = useState({
    strategy_id: '',
    symbols: 'sz000001',
    start_date: '2020-01-01',
    end_date: '2023-12-31',
    init_capital: 1000000,
  });
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    Promise.all([api.getStrategies(), api.getBacktests()])
      .then(([strData, runData]) => {
        setStrategies(strData.strategies ?? []);
        setRuns(runData.runs ?? []);
      })
      .catch(console.error);
  }, []);

  async function handleRun(e) {
    e.preventDefault();
    setError('');
    setRunning(true);
    try {
      const payload = {
        strategy_id: form.strategy_id ? Number(form.strategy_id) : undefined,
        symbols: form.symbols
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean),
        start_date: form.start_date,
        end_date: form.end_date,
        init_capital: Number(form.init_capital),
      };
      const result = await api.runBacktest(payload);
      navigate(`/backtests/${result.id}`);
    } catch (e) {
      setError(e.message);
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* Run form */}
      <div className="bg-white rounded-lg shadow p-6">
        <h1 className="text-xl font-semibold mb-4">运行回测</h1>
        <form onSubmit={handleRun} className="space-y-4">
          <label className="block text-sm font-medium">
            选择策略
            <select
              className="mt-1 block w-full border rounded px-3 py-2 text-sm"
              value={form.strategy_id}
              onChange={(e) => set('strategy_id', e.target.value)}
              required
            >
              <option value="">— 请选择 —</option>
              {strategies.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.class_name})
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm font-medium">
            股票代码（逗号分隔）
            <input
              className="mt-1 block w-full border rounded px-3 py-2 text-sm font-mono"
              value={form.symbols}
              onChange={(e) => set('symbols', e.target.value)}
              placeholder="sz000001,sh600000"
              required
            />
          </label>
          <div className="grid grid-cols-2 gap-3">
            <label className="block text-sm font-medium">
              开始日期
              <input
                type="date"
                className="mt-1 block w-full border rounded px-3 py-2 text-sm"
                value={form.start_date}
                onChange={(e) => set('start_date', e.target.value)}
                required
              />
            </label>
            <label className="block text-sm font-medium">
              结束日期
              <input
                type="date"
                className="mt-1 block w-full border rounded px-3 py-2 text-sm"
                value={form.end_date}
                onChange={(e) => set('end_date', e.target.value)}
                required
              />
            </label>
          </div>
          <label className="block text-sm font-medium">
            初始资金（元）
            <input
              type="number"
              className="mt-1 block w-full border rounded px-3 py-2 text-sm"
              value={form.init_capital}
              onChange={(e) => set('init_capital', e.target.value)}
              min={10000}
              step={10000}
              required
            />
          </label>
          {error && <p className="text-red-500 text-sm">{error}</p>}
          <button
            type="submit"
            disabled={running}
            className="w-full py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 text-sm font-medium"
          >
            {running ? '运行中…' : '开始回测'}
          </button>
        </form>
      </div>

      {/* History */}
      <div className="bg-white rounded-lg shadow p-6">
        <h2 className="text-lg font-semibold mb-4">历史记录</h2>
        {runs.length === 0 ? (
          <p className="text-gray-400 text-center py-8">暂无回测记录</p>
        ) : (
          <div className="space-y-2 max-h-[500px] overflow-y-auto">
            {runs.map((r) => (
              <div
                key={r.id}
                className="flex justify-between items-center p-3 rounded border hover:bg-gray-50 cursor-pointer"
                onClick={() => navigate(`/backtests/${r.id}`)}
              >
                <div>
                  <p className="text-sm font-medium">{r.strategy_name}</p>
                  <p className="text-xs text-gray-400">
                    {r.start_date} ~ {r.end_date}
                  </p>
                </div>
                <div className="text-right">
                  <span
                    className={`text-xs px-2 py-0.5 rounded-full ${
                      r.status === 'done'
                        ? 'bg-green-100 text-green-700'
                        : r.status === 'failed'
                          ? 'bg-red-100 text-red-600'
                          : 'bg-yellow-100 text-yellow-700'
                    }`}
                  >
                    {r.status}
                  </span>
                  {r.annual_return != null && (
                    <p
                      className={`text-sm font-mono mt-0.5 ${
                        r.annual_return >= 0 ? 'text-green-600' : 'text-red-500'
                      }`}
                    >
                      {(r.annual_return * 100).toFixed(1)}%/年
                    </p>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
