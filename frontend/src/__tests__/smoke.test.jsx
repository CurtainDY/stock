import React from 'react';
import { render, screen } from '@testing-library/react';
import App from '../App';

test('renders nav bar with app name', () => {
  render(<App />);
  expect(screen.getByText('A股回测')).toBeInTheDocument();
});

test('renders strategies and backtest nav links', () => {
  render(<App />);
  expect(screen.getAllByText('策略管理').length).toBeGreaterThan(0);
  expect(screen.getByText('运行回测')).toBeInTheDocument();
});
