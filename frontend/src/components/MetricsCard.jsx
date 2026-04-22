import React from 'react';

function fmt(v, isPct = true) {
  if (v == null) return '—';
  return isPct ? `${(v * 100).toFixed(2)}%` : v.toFixed(4);
}

export default function MetricsCard({ run }) {
  const metrics = [
    { label: '年化收益', value: fmt(run.annual_return), positive: (run.annual_return ?? 0) >= 0 },
    { label: '最大回撤', value: fmt(run.max_drawdown), positive: false },
    { label: '夏普比率', value: fmt(run.sharpe_ratio, false), positive: (run.sharpe_ratio ?? 0) > 0 },
    { label: '胜率', value: fmt(run.win_rate), positive: (run.win_rate ?? 0) > 0.5 },
    { label: 'Calmar', value: fmt(run.calmar_ratio, false), positive: (run.calmar_ratio ?? 0) > 0 },
    { label: '总收益', value: fmt(run.total_return), positive: (run.total_return ?? 0) >= 0 },
  ];
  return (
    <div className="grid grid-cols-3 gap-4" data-testid="metrics-card">
      {metrics.map(({ label, value, positive }) => (
        <div key={label} className="bg-white rounded-lg shadow p-4 text-center">
          <p className="text-xs text-gray-400 mb-1">{label}</p>
          <p className={`text-2xl font-bold ${positive ? 'text-green-600' : 'text-red-500'}`}>
            {value}
          </p>
        </div>
      ))}
    </div>
  );
}
