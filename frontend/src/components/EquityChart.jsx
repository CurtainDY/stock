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
