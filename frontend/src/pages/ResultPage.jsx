import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import EquityChart from '../components/EquityChart';
import MetricsCard from '../components/MetricsCard';

export default function ResultPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [run, setRun] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api
      .getBacktest(id)
      .then(setRun)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) return <p className="text-gray-400 text-center py-12">加载中…</p>;
  if (error) return <p className="text-red-500 text-center py-12">{error}</p>;
  if (!run) return null;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4 flex-wrap">
        <button
          onClick={() => navigate(-1)}
          className="text-gray-400 hover:text-gray-600 text-sm"
        >
          ← 返回
        </button>
        <h1 className="text-xl font-semibold">{run.strategy_name}</h1>
        <span
          className={`text-xs px-2 py-0.5 rounded-full ${
            run.status === 'done'
              ? 'bg-green-100 text-green-700'
              : run.status === 'failed'
                ? 'bg-red-100 text-red-600'
                : 'bg-yellow-100 text-yellow-700'
          }`}
        >
          {run.status}
        </span>
      </div>

      {/* Metadata */}
      <div className="text-sm text-gray-500">
        {run.start_date} ~ {run.end_date}&nbsp;|&nbsp;
        {run.symbols?.join(', ')}&nbsp;|&nbsp;初始资金 ¥
        {run.init_capital?.toLocaleString()}
      </div>

      {/* Error block */}
      {run.status === 'failed' && (
        <div className="bg-red-50 border border-red-200 rounded p-4 text-sm text-red-700 font-mono whitespace-pre-wrap">
          {run.error_msg || '未知错误'}
        </div>
      )}

      {/* Metrics + chart */}
      {run.status === 'done' && (
        <>
          <MetricsCard run={run} />
          {run.equity_curve?.length > 0 && <EquityChart data={run.equity_curve} />}
        </>
      )}
    </div>
  );
}
