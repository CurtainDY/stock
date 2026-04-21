from dataclasses import dataclass
from datetime import date
from typing import Callable, Type

from dateutil.relativedelta import relativedelta

from backtest.analytics import Metrics
from backtest.strategy import Strategy


@dataclass
class WalkForwardConfig:
    start_date: date
    end_date: date
    train_years: int
    test_years: int
    mode: str = "expanding"          # "expanding" 或 "rolling"
    min_annual_return: float = 0.10
    min_sharpe: float = 0.80
    max_drawdown: float = 0.30
    min_win_rate: float = 0.45


@dataclass
class WFWindow:
    window_id: int
    train_start: date
    train_end: date
    test_start: date
    test_end: date


@dataclass
class WalkForwardResult:
    num_windows: int
    test_metrics: list[Metrics]
    avg_annual_return: float
    avg_max_drawdown: float
    avg_sharpe: float
    passes_filter: bool


class WalkForwardValidator:
    def __init__(self, config: WalkForwardConfig):
        self._cfg = config

    def windows(self) -> list[WFWindow]:
        """生成 Walk-Forward 窗口列表。"""
        result = []
        cfg = self._cfg
        test_delta = relativedelta(years=cfg.test_years)
        train_delta = relativedelta(years=cfg.train_years)

        window_id = 0
        test_start = cfg.start_date + train_delta
        while True:
            test_end = test_start + test_delta
            if test_end > cfg.end_date:
                break

            if cfg.mode == "expanding":
                train_start = cfg.start_date
            else:  # rolling
                train_start = test_start - train_delta

            result.append(WFWindow(
                window_id=window_id,
                train_start=train_start,
                train_end=test_start,
                test_start=test_start,
                test_end=test_end,
            ))
            window_id += 1
            test_start = test_end

        return result

    def passes_filter(self, metrics: Metrics) -> bool:
        cfg = self._cfg
        return (
            metrics.annual_return >= cfg.min_annual_return
            and metrics.sharpe_ratio >= cfg.min_sharpe
            and metrics.max_drawdown <= cfg.max_drawdown
            and metrics.win_rate >= cfg.min_win_rate
        )

    def run(
        self,
        strategy_cls: Type[Strategy],
        runner_factory: Callable,  # (window: WFWindow, phase: str) -> BacktestRunner
    ) -> WalkForwardResult:
        """
        对每个窗口运行训练期和测试期回测，返回测试期汇总指标。
        runner_factory(window, phase) 返回已配置好日期的 BacktestRunner。
        phase 为 "train" 或 "test"。
        """
        windows = self.windows()
        test_metrics: list[Metrics] = []

        for window in windows:
            train_runner = runner_factory(window, "train")
            train_runner.run(strategy_cls())

            test_runner = runner_factory(window, "test")
            metrics = test_runner.run(strategy_cls())
            test_metrics.append(metrics)

        if not test_metrics:
            return WalkForwardResult(
                num_windows=0,
                test_metrics=[],
                avg_annual_return=0.0,
                avg_max_drawdown=0.0,
                avg_sharpe=0.0,
                passes_filter=False,
            )

        avg_annual = sum(m.annual_return for m in test_metrics) / len(test_metrics)
        avg_dd = sum(m.max_drawdown for m in test_metrics) / len(test_metrics)
        avg_sharpe = sum(m.sharpe_ratio for m in test_metrics) / len(test_metrics)

        summary_metrics = Metrics(
            total_return=avg_annual,
            annual_return=avg_annual,
            max_drawdown=avg_dd,
            sharpe_ratio=avg_sharpe,
            win_rate=sum(m.win_rate for m in test_metrics) / len(test_metrics),
            calmar_ratio=avg_annual / avg_dd if avg_dd > 0 else 0.0,
            equity_curve=[1.0],
        )

        return WalkForwardResult(
            num_windows=len(test_metrics),
            test_metrics=test_metrics,
            avg_annual_return=avg_annual,
            avg_max_drawdown=avg_dd,
            avg_sharpe=avg_sharpe,
            passes_filter=self.passes_filter(summary_metrics),
        )
