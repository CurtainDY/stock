from abc import ABC, abstractmethod
from backtest.types import Bar, Order, PortfolioSnapshot


class Strategy(ABC):
    """所有策略必须继承此类并实现 on_bar。"""

    @abstractmethod
    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        """
        接收新 Bar，返回订单列表（空列表 = 不操作）。
        bar: 当前 Bar（只有当前及之前信息，禁止前视）
        portfolio: 当前持仓快照（只读）
        """
        ...

    def on_start(self) -> None:
        """回测开始时调用（可选覆盖）"""

    def on_end(self) -> None:
        """回测结束时调用（可选覆盖）"""
