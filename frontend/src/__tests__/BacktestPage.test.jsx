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
