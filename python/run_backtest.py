#!/usr/bin/env python3
"""
回测 CLI 入口。被 api-server 以子进程方式调用。

用法：
  python run_backtest.py \
    --strategy MACrossStrategy \
    --params '{"fast":5,"slow":20}' \
    --symbols sz000001,sh600000 \
    --start 2020-01-01 \
    --end 2023-12-31 \
    --capital 1000000 \
    --grpc-host localhost \
    --grpc-port 50051

输出到 stdout（JSON）：
  {"status":"done","annual_return":0.15,"max_drawdown":0.08,...,"equity_curve":[1.0,...]}

错误时输出：
  {"status":"failed","error":"..."}
"""
import argparse
import importlib
import json
import sys
import traceback
from datetime import date


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--strategy", required=True, help="策略类名")
    parser.add_argument("--params", default="{}", help="策略参数 JSON")
    parser.add_argument("--symbols", required=True, help="股票代码，逗号分隔")
    parser.add_argument("--start", required=True, help="开始日期 YYYY-MM-DD")
    parser.add_argument("--end", required=True, help="结束日期 YYYY-MM-DD")
    parser.add_argument("--capital", type=float, default=1_000_000)
    parser.add_argument("--grpc-host", default="localhost")
    parser.add_argument("--grpc-port", type=int, default=50051)
    args = parser.parse_args()

    try:
        result = run_backtest(args)
        print(json.dumps(result))
    except Exception as e:
        print(json.dumps({"status": "failed", "error": traceback.format_exc()}))
        sys.exit(1)


def run_backtest(args) -> dict:
    from backtest.client import BacktestClient
    from backtest.runner import BacktestConfig, BacktestRunner

    # 动态加载策略类
    strategy_cls = _load_strategy(args.strategy)
    params = json.loads(args.params)

    symbols = [s.strip() for s in args.symbols.split(",")]
    start = date.fromisoformat(args.start)
    end = date.fromisoformat(args.end)

    config = BacktestConfig(
        symbols=symbols,
        start_date=start,
        end_date=end,
        init_capital=args.capital,
    )

    with BacktestClient(host=args.grpc_host, port=args.grpc_port) as client:
        session_id = client.new_session_id()
        runner = BacktestRunner(stub=client.stub, config=config, session_id=session_id)
        strategy = strategy_cls(**params)
        metrics = runner.run(strategy)

    return {
        "status": "done",
        "annual_return": metrics.annual_return,
        "max_drawdown": metrics.max_drawdown,
        "sharpe_ratio": metrics.sharpe_ratio,
        "win_rate": metrics.win_rate,
        "calmar_ratio": metrics.calmar_ratio,
        "total_return": metrics.total_return,
        "equity_curve": metrics.equity_curve,
    }


def _load_strategy(class_name: str):
    """
    按类名查找策略类，先在 strategies/ 包中搜索，再在 backtest/ 中搜索。
    支持 "MACrossStrategy" 或 "strategies.ma_cross.MACrossStrategy" 格式。
    """
    if "." in class_name:
        module_path, cls = class_name.rsplit(".", 1)
        mod = importlib.import_module(module_path)
        return getattr(mod, cls)

    # 简短名称：遍历 strategies 子模块
    import pkgutil
    import strategies as strat_pkg

    for importer, modname, ispkg in pkgutil.iter_modules(strat_pkg.__path__):
        mod = importlib.import_module(f"strategies.{modname}")
        if hasattr(mod, class_name):
            return getattr(mod, class_name)

    raise ValueError(f"Strategy class {class_name!r} not found in strategies/")


if __name__ == "__main__":
    main()
