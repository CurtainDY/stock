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
