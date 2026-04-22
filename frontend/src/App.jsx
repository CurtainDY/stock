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
