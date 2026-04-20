import math
from dataclasses import dataclass


@dataclass
class Metrics:
    total_return: float
    annual_return: float
    max_drawdown: float
    sharpe_ratio: float
    win_rate: float
    calmar_ratio: float
    equity_curve: list[float]


def compute_metrics(equity_curve: list[float], trading_days_per_year: int = 242) -> Metrics:
    n = len(equity_curve)
    if n < 2:
        return Metrics(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, equity_curve)

    daily_returns = [
        (equity_curve[i] - equity_curve[i - 1]) / equity_curve[i - 1]
        for i in range(1, n)
    ]

    total_return = (equity_curve[-1] - equity_curve[0]) / equity_curve[0]

    years = (n - 1) / trading_days_per_year
    if years > 0 and equity_curve[0] > 0:
        annual_return = (equity_curve[-1] / equity_curve[0]) ** (1.0 / years) - 1.0
    else:
        annual_return = 0.0

    max_drawdown = _max_drawdown(equity_curve)
    sharpe_ratio = _sharpe(daily_returns, trading_days_per_year)

    wins = sum(1 for r in daily_returns if r > 0)
    win_rate = wins / len(daily_returns) if daily_returns else 0.0

    calmar_ratio = annual_return / max_drawdown if max_drawdown > 0 else 0.0

    return Metrics(
        total_return=total_return,
        annual_return=annual_return,
        max_drawdown=max_drawdown,
        sharpe_ratio=sharpe_ratio,
        win_rate=win_rate,
        calmar_ratio=calmar_ratio,
        equity_curve=list(equity_curve),
    )


def _max_drawdown(equity: list[float]) -> float:
    peak = equity[0]
    max_dd = 0.0
    for v in equity:
        if v > peak:
            peak = v
        dd = (peak - v) / peak if peak > 0 else 0.0
        if dd > max_dd:
            max_dd = dd
    return max_dd


def _sharpe(daily_returns: list[float], trading_days_per_year: int) -> float:
    if not daily_returns:
        return 0.0
    n = len(daily_returns)
    mean = sum(daily_returns) / n
    variance = sum((r - mean) ** 2 for r in daily_returns) / n
    std = math.sqrt(variance)
    if std == 0:
        return 0.0
    return mean / std * math.sqrt(trading_days_per_year)
