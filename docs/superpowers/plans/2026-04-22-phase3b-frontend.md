# Phase 3b: React Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 React + TailwindCSS + ECharts 构建前端，提供策略管理、回测触发、结果可视化三个页面，通过 Vite 代理连接 api-server（:8080）。

**Architecture:** `frontend/` 是独立目录（非 Go 模块），使用 Vite 作为构建工具，通过 `/v1/*` 代理转发到 api-server。三个主页面（策略管理、运行回测、回测结果），共享 API 客户端和两个展示组件（净值曲线图、指标卡片）。

**Tech Stack:** React 18, React Router v6, TailwindCSS v3, ECharts 5 + echarts-for-react, Vite 5, Vitest 2 + @testing-library/react

---

## 参考资料

- API 端点：`GET/POST /v1/strategies`，`GET/PUT/DELETE /v1/strategies/:id`，`POST/GET /v1/backtests`，`GET /v1/backtests/:id`，`GET /v1/symbols`
- api-server 默认端口：8080
- 系统规格：`docs/superpowers/specs/2026-04-16-system-spec.md`（前端技术栈约束：React + TailwindCSS + ECharts）

---

## 文件结构

```
frontend/
├── package.json
├── vite.config.js
├── tailwind.config.js
├── postcss.config.js
├── index.html
└── src/
    ├── index.css                    # Tailwind 指令
    ├── setupTests.js                # jest-dom + echarts mock
    ├── main.jsx                     # ReactDOM 入口
    ├── App.jsx                      # BrowserRouter + Nav + Routes
    ├── api/
    │   └── client.js                # fetch 封装，8个方法
    ├── components/
    │   ├── EquityChart.jsx          # ECharts 净值折线图
    │   └── MetricsCard.jsx          # 6格指标展示
    ├── pages/
    │   ├── StrategiesPage.jsx       # 策略 CRUD（表格 + Modal）
    │   ├── BacktestPage.jsx         # 回测表单 + 历史列表
    │   └── ResultPage.jsx           # 单次回测详情
    └── __tests__/
        ├── api.test.js
        ├── StrategiesPage.test.jsx
        ├── BacktestPage.test.jsx
        └── ResultPage.test.jsx
```

---

## Task 1: 项目初始化 — Vite + React + Tailwind + 测试框架

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.js`
- Create: `frontend/tailwind.config.js`
- Create: `frontend/postcss.config.js`
- Create: `frontend/index.html`
- Create: `frontend/src/index.css`
- Create: `frontend/src/setupTests.js`
- Create: `frontend/src/main.jsx`
- Create: `frontend/src/App.jsx`（骨架）
- Create: `frontend/src/__tests__/smoke.test.jsx`

- [ ] **Step 1: 创建 `frontend/package.json`**

```json
{
  "name": "stock-frontend",
  "version": "0.0.1",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "echarts": "^5.5.0",
    "echarts-for-react": "^3.0.2",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.2"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.4.6",
    "@testing-library/react": "^16.0.0",
    "@testing-library/user-event": "^14.5.2",
    "@vitejs/plugin-react": "^4.3.1",
    "autoprefixer": "^10.4.20",
    "jsdom": "^24.1.1",
    "postcss": "^8.4.40",
    "tailwindcss": "^3.4.9",
    "vite": "^5.4.0",
    "vitest": "^2.0.5"
  }
}
```

- [ ] **Step 2: 安装依赖**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm install
```

Expected: `node_modules/` 创建，无报错

- [ ] **Step 3: 创建 `frontend/vite.config.js`**

```javascript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/v1': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/setupTests.js'],
  },
});
```

- [ ] **Step 4: 创建 `frontend/tailwind.config.js`**

```javascript
export default {
  content: ['./index.html', './src/**/*.{js,jsx}'],
  theme: { extend: {} },
  plugins: [],
};
```

- [ ] **Step 5: 创建 `frontend/postcss.config.js`**

```javascript
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
```

- [ ] **Step 6: 创建 `frontend/index.html`**

```html
<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>A股回测系统</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.jsx"></script>
  </body>
</html>
```

- [ ] **Step 7: 创建 `frontend/src/index.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 8: 创建 `frontend/src/setupTests.js`**

```javascript
import '@testing-library/jest-dom';
import { vi } from 'vitest';

// ECharts 在 jsdom 中无法渲染 canvas，mock 掉
vi.mock('echarts-for-react', () => ({
  default: () => null,
}));
```

- [ ] **Step 9: 创建骨架页面文件（让 App.jsx 可以 import）**

创建 `frontend/src/pages/StrategiesPage.jsx`：

```jsx
export default function StrategiesPage() {
  return <div data-testid="strategies-page">策略管理</div>;
}
```

创建 `frontend/src/pages/BacktestPage.jsx`：

```jsx
export default function BacktestPage() {
  return <div data-testid="backtest-page">运行回测</div>;
}
```

创建 `frontend/src/pages/ResultPage.jsx`：

```jsx
export default function ResultPage() {
  return <div data-testid="result-page">回测结果</div>;
}
```

- [ ] **Step 10: 创建 `frontend/src/App.jsx`**

```jsx
import React from 'react';
import { BrowserRouter, NavLink, Route, Routes } from 'react-router-dom';
import StrategiesPage from './pages/StrategiesPage';
import BacktestPage from './pages/BacktestPage';
import ResultPage from './pages/ResultPage';

function Nav() {
  const linkCls = ({ isActive }) =>
    `px-4 py-2 rounded text-sm font-medium transition-colors ${
      isActive ? 'bg-blue-600 text-white' : 'text-gray-600 hover:text-blue-600'
    }`;
  return (
    <nav className="bg-white border-b shadow-sm px-6 py-3 flex gap-4 items-center">
      <span className="font-bold text-lg text-blue-700 mr-4">A股回测</span>
      <NavLink to="/strategies" className={linkCls}>策略管理</NavLink>
      <NavLink to="/backtest" className={linkCls}>运行回测</NavLink>
    </nav>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-50">
        <Nav />
        <main className="max-w-6xl mx-auto p-6">
          <Routes>
            <Route path="/" element={<StrategiesPage />} />
            <Route path="/strategies" element={<StrategiesPage />} />
            <Route path="/backtest" element={<BacktestPage />} />
            <Route path="/backtests/:id" element={<ResultPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}
```

- [ ] **Step 11: 创建 `frontend/src/main.jsx`**

```jsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

- [ ] **Step 12: 写冒烟测试 `frontend/src/__tests__/smoke.test.jsx`**

```jsx
import React from 'react';
import { render, screen } from '@testing-library/react';
import App from '../App';

test('renders nav bar with app name', () => {
  render(<App />);
  expect(screen.getByText('A股回测')).toBeInTheDocument();
});

test('renders strategies and backtest nav links', () => {
  render(<App />);
  expect(screen.getByText('策略管理')).toBeInTheDocument();
  expect(screen.getByText('运行回测')).toBeInTheDocument();
});
```

- [ ] **Step 13: 运行测试验证配置**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test
```

Expected:
```
✓ smoke.test.jsx (2 tests)
Test Files  1 passed
Tests       2 passed
```

- [ ] **Step 14: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add frontend/
git commit -m "feat: initialize React frontend with Vite, Tailwind, ECharts"
```

---

## Task 2: API 客户端 + 组件（EquityChart、MetricsCard）

**Files:**
- Create: `frontend/src/api/client.js`
- Create: `frontend/src/components/EquityChart.jsx`
- Create: `frontend/src/components/MetricsCard.jsx`
- Create: `frontend/src/__tests__/api.test.js`

- [ ] **Step 1: 写 API 客户端测试 `frontend/src/__tests__/api.test.js`**

```javascript
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
```

- [ ] **Step 2: 运行测试验证失败（TDD）**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/api.test.js
```

Expected: FAIL with "Cannot find module '../api/client'"

- [ ] **Step 3: 创建 `frontend/src/api/client.js`**

```javascript
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
```

- [ ] **Step 4: 运行 API 测试验证通过**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/api.test.js
```

Expected: 4 tests PASS

- [ ] **Step 5: 创建 `frontend/src/components/MetricsCard.jsx`**

```jsx
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
```

- [ ] **Step 6: 创建 `frontend/src/components/EquityChart.jsx`**

```jsx
import React from 'react';
import ReactECharts from 'echarts-for-react';

export default function EquityChart({ data }) {
  const option = {
    title: { text: '净值曲线', left: 'left', textStyle: { fontSize: 14 } },
    tooltip: {
      trigger: 'axis',
      formatter: (params) => `净值: ${Number(params[0].value).toFixed(4)}`,
    },
    xAxis: {
      type: 'category',
      data: data.map((_, i) => i + 1),
      axisLabel: { show: false },
    },
    yAxis: {
      type: 'value',
      scale: true,
      axisLabel: { formatter: (v) => Number(v).toFixed(2) },
    },
    series: [
      {
        data,
        type: 'line',
        smooth: false,
        lineStyle: { color: '#3b82f6', width: 2 },
        areaStyle: { color: 'rgba(59,130,246,0.08)' },
        symbol: 'none',
      },
    ],
    grid: { left: 60, right: 20, top: 40, bottom: 20 },
  };
  return (
    <div className="bg-white rounded-lg shadow p-4" data-testid="equity-chart">
      <ReactECharts option={option} style={{ height: 300 }} />
    </div>
  );
}
```

- [ ] **Step 7: 运行全部测试**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test
```

Expected: 6 tests PASS (2 smoke + 4 api)

- [ ] **Step 8: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add frontend/
git commit -m "feat: add API client and shared components (EquityChart, MetricsCard)"
```

---

## Task 3: 策略管理页面

**Files:**
- Modify: `frontend/src/pages/StrategiesPage.jsx`（替换骨架）
- Create: `frontend/src/__tests__/StrategiesPage.test.jsx`

- [ ] **Step 1: 写失败测试 `frontend/src/__tests__/StrategiesPage.test.jsx`**

```jsx
import React from 'react';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import StrategiesPage from '../pages/StrategiesPage';

const MOCK_STRATEGIES = [
  {
    id: 1,
    name: 'MA Cross',
    class_name: 'MACrossStrategy',
    params: { fast: 5, slow: 20 },
    created_at: '2024-01-15T00:00:00Z',
  },
];

function mockFetchStrategies(strategies = MOCK_STRATEGIES) {
  global.fetch = vi.fn(() =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ strategies, total: strategies.length }),
    }),
  );
}

beforeEach(() => mockFetchStrategies());
afterEach(() => vi.restoreAllMocks());

function renderPage() {
  return render(<BrowserRouter><StrategiesPage /></BrowserRouter>);
}

test('renders strategy table with data', async () => {
  renderPage();
  await waitFor(() => expect(screen.getByText('MA Cross')).toBeInTheDocument());
  expect(screen.getByText('MACrossStrategy')).toBeInTheDocument();
  expect(screen.getByText('2024-01-15')).toBeInTheDocument();
});

test('shows empty state when no strategies', async () => {
  mockFetchStrategies([]);
  renderPage();
  await waitFor(() =>
    expect(screen.getByText(/暂无策略/)).toBeInTheDocument(),
  );
});

test('opens create modal on button click', async () => {
  renderPage();
  await waitFor(() => screen.getByText('MA Cross'));
  fireEvent.click(screen.getByText('+ 新建策略'));
  expect(screen.getByText('新建策略')).toBeInTheDocument();
  expect(screen.getByPlaceholderText('MACrossStrategy')).toBeInTheDocument();
});

test('closes modal on 取消 click', async () => {
  renderPage();
  await waitFor(() => screen.getByText('MA Cross'));
  fireEvent.click(screen.getByText('+ 新建策略'));
  fireEvent.click(screen.getByText('取消'));
  expect(screen.queryByText('新建策略')).not.toBeInTheDocument();
});
```

- [ ] **Step 2: 运行测试验证失败**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/StrategiesPage.test.jsx
```

Expected: FAIL — 骨架组件没有这些内容

- [ ] **Step 3: 实现 `frontend/src/pages/StrategiesPage.jsx`**

```jsx
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
  // modal: null | { mode: 'create' } | { mode: 'edit', strategy: { ...s, params: string } }
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
```

- [ ] **Step 4: 运行测试验证通过**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/StrategiesPage.test.jsx
```

Expected: 4 tests PASS

- [ ] **Step 5: 运行全部测试**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test
```

Expected: 10 tests PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add frontend/
git commit -m "feat: add strategies management page with CRUD"
```

---

## Task 4: 回测运行页面

**Files:**
- Modify: `frontend/src/pages/BacktestPage.jsx`（替换骨架）
- Create: `frontend/src/__tests__/BacktestPage.test.jsx`

- [ ] **Step 1: 写失败测试 `frontend/src/__tests__/BacktestPage.test.jsx`**

```jsx
import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import BacktestPage from '../pages/BacktestPage';

const MOCK_STRATEGIES = [
  { id: 1, name: 'MA Cross', class_name: 'MACrossStrategy', params: {} },
];

const MOCK_RUNS = [
  {
    id: 1,
    strategy_name: 'MA Cross',
    status: 'done',
    start_date: '2020-01-01',
    end_date: '2023-12-31',
    annual_return: 0.15,
  },
];

function mockFetch({ strategies = MOCK_STRATEGIES, runs = MOCK_RUNS } = {}) {
  global.fetch = vi.fn((url) => {
    if (url.includes('/v1/strategies'))
      return Promise.resolve({
        ok: true, status: 200,
        json: () => Promise.resolve({ strategies }),
      });
    if (url.includes('/v1/backtests'))
      return Promise.resolve({
        ok: true, status: 200,
        json: () => Promise.resolve({ runs }),
      });
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
  });
}

beforeEach(() => mockFetch());
afterEach(() => vi.restoreAllMocks());

function renderPage() {
  return render(<MemoryRouter><BacktestPage /></MemoryRouter>);
}

test('renders form and history sections', async () => {
  renderPage();
  await waitFor(() => expect(screen.getByText('运行回测')).toBeInTheDocument());
  expect(screen.getByText('历史记录')).toBeInTheDocument();
});

test('loads and displays strategies in select', async () => {
  renderPage();
  await waitFor(() =>
    expect(screen.getByText('MA Cross (MACrossStrategy)')).toBeInTheDocument(),
  );
});

test('displays history run with annual return', async () => {
  renderPage();
  await waitFor(() => expect(screen.getByText('15.0%/年')).toBeInTheDocument());
});

test('shows empty history when no runs', async () => {
  mockFetch({ runs: [] });
  renderPage();
  await waitFor(() => expect(screen.getByText('暂无回测记录')).toBeInTheDocument());
});
```

- [ ] **Step 2: 运行测试验证失败**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/BacktestPage.test.jsx
```

Expected: FAIL

- [ ] **Step 3: 实现 `frontend/src/pages/BacktestPage.jsx`**

```jsx
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
```

- [ ] **Step 4: 运行测试验证通过**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/BacktestPage.test.jsx
```

Expected: 4 tests PASS

- [ ] **Step 5: 运行全部测试**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test
```

Expected: 14 tests PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add frontend/
git commit -m "feat: add backtest run form and history list page"
```

---

## Task 5: 回测结果页面（指标 + 净值曲线图）

**Files:**
- Modify: `frontend/src/pages/ResultPage.jsx`（替换骨架）
- Create: `frontend/src/__tests__/ResultPage.test.jsx`

- [ ] **Step 1: 写失败测试 `frontend/src/__tests__/ResultPage.test.jsx`**

```jsx
import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ResultPage from '../pages/ResultPage';

const MOCK_RUN = {
  id: 1,
  strategy_name: 'MA Cross',
  status: 'done',
  start_date: '2020-01-01',
  end_date: '2023-12-31',
  symbols: ['sz000001'],
  init_capital: 1000000,
  annual_return: 0.15,
  max_drawdown: 0.08,
  sharpe_ratio: 1.2,
  win_rate: 0.55,
  calmar_ratio: 1.875,
  total_return: 0.72,
  equity_curve: [1.0, 1.02, 1.05, 0.98, 1.15],
};

beforeEach(() => {
  global.fetch = vi.fn(() =>
    Promise.resolve({
      ok: true, status: 200,
      json: () => Promise.resolve(MOCK_RUN),
    }),
  );
});
afterEach(() => vi.restoreAllMocks());

function renderResult() {
  return render(
    <MemoryRouter initialEntries={['/backtests/1']}>
      <Routes>
        <Route path="/backtests/:id" element={<ResultPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

test('renders strategy name and status badge', async () => {
  renderResult();
  await waitFor(() => expect(screen.getByText('MA Cross')).toBeInTheDocument());
  expect(screen.getByText('done')).toBeInTheDocument();
});

test('renders date range and symbols', async () => {
  renderResult();
  await waitFor(() => expect(screen.getByText(/2020-01-01/)).toBeInTheDocument());
  expect(screen.getByText(/sz000001/)).toBeInTheDocument();
});

test('renders metrics card with annual return', async () => {
  renderResult();
  await waitFor(() => expect(screen.getByText('年化收益')).toBeInTheDocument());
  expect(screen.getByText('15.00%')).toBeInTheDocument();
  expect(screen.getByText('最大回撤')).toBeInTheDocument();
  expect(screen.getByText('8.00%')).toBeInTheDocument();
});

test('renders equity chart placeholder', async () => {
  renderResult();
  await waitFor(() => expect(screen.getByTestId('equity-chart')).toBeInTheDocument());
});

test('renders error message for failed run', async () => {
  global.fetch = vi.fn(() =>
    Promise.resolve({
      ok: true, status: 200,
      json: () =>
        Promise.resolve({
          ...MOCK_RUN,
          status: 'failed',
          error_msg: 'strategy not found',
          equity_curve: null,
          annual_return: null,
        }),
    }),
  );
  renderResult();
  await waitFor(() => expect(screen.getByText('strategy not found')).toBeInTheDocument());
});
```

- [ ] **Step 2: 运行测试验证失败**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/ResultPage.test.jsx
```

Expected: FAIL

- [ ] **Step 3: 实现 `frontend/src/pages/ResultPage.jsx`**

```jsx
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
```

- [ ] **Step 4: 运行测试验证通过**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test -- --reporter=verbose src/__tests__/ResultPage.test.jsx
```

Expected: 5 tests PASS

- [ ] **Step 5: 运行全部测试**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm test
```

Expected: 19 tests PASS

- [ ] **Step 6: 验证 Vite 构建无报错**

```bash
cd /Users/parsedong/workSpace/stock/frontend
npm run build
```

Expected: `dist/` 目录生成，无 TypeScript/Vite 报错

- [ ] **Step 7: Commit**

```bash
cd /Users/parsedong/workSpace/stock
git add frontend/
git commit -m "feat: add backtest result page with metrics and equity curve chart"
```

---

## 自检：Spec 覆盖确认

| Spec 要求 | 对应任务 |
|----------|---------|
| React + TailwindCSS + ECharts（强制）| Task 1 |
| 不使用 Vue / Ant Design | ✓ 未引入 |
| 策略管理（CRUD）| Task 3 StrategiesPage |
| 回测触发界面 | Task 4 BacktestPage |
| 绩效指标展示（6个指标）| Task 2 MetricsCard + Task 5 ResultPage |
| 净值曲线图 | Task 2 EquityChart + Task 5 ResultPage |
| 前端不直接访问 DB / gRPC | ✓ 全部经过 api-server HTTP |
| CORS 支持（api-server 已开 `WithCors("*")`）| ✓ Task 3a 已实现 |
