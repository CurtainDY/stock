import '@testing-library/jest-dom';
import { vi } from 'vitest';

// ECharts cannot render canvas in jsdom — mock it
vi.mock('echarts-for-react', () => ({
  default: () => null,
}));
