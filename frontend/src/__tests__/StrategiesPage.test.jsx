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
