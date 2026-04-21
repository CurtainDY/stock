from collections import deque
from backtest.types import Bar, Order, OrderSide, PortfolioSnapshot
from backtest.strategy import Strategy


class MACrossStrategy(Strategy):
    """
    双均线金叉/死叉策略（使用复权价格）。
    - 快线上穿慢线（金叉）→ 全仓买入
    - 快线下穿慢线（死叉）→ 全部卖出
    """

    def __init__(self, fast: int = 5, slow: int = 20, position_pct: float = 0.95):
        self.fast = fast
        self.slow = slow
        self.position_pct = position_pct
        self._prices: dict[str, deque] = {}
        self._prev_fast: dict[str, float] = {}
        self._prev_slow: dict[str, float] = {}

    def on_bar(self, bar: Bar, portfolio: PortfolioSnapshot) -> list[Order]:
        sym = bar.symbol
        price = bar.adj_close  # 使用复权价格

        if sym not in self._prices:
            self._prices[sym] = deque(maxlen=self.slow)

        self._prices[sym].append(price)

        prices = list(self._prices[sym])
        if len(prices) < self.slow:
            return []  # 数据不足，等待

        fast_ma = sum(prices[-self.fast:]) / self.fast
        slow_ma = sum(prices) / self.slow

        prev_fast = self._prev_fast.get(sym, fast_ma)
        prev_slow = self._prev_slow.get(sym, slow_ma)
        self._prev_fast[sym] = fast_ma
        self._prev_slow[sym] = slow_ma

        orders = []
        pos = portfolio.positions.get(sym)

        # 金叉：快线从下穿过慢线
        if prev_fast <= prev_slow and fast_ma > slow_ma:
            if not pos or pos.quantity == 0:
                qty = int(portfolio.cash * self.position_pct / price / 100) * 100
                if qty >= 100:
                    orders.append(Order(symbol=sym, side=OrderSide.BUY, quantity=qty))

        # 死叉：快线从上穿过慢线
        elif prev_fast >= prev_slow and fast_ma < slow_ma:
            if pos and pos.quantity > 0:
                orders.append(Order(symbol=sym, side=OrderSide.SELL, quantity=pos.quantity))

        return orders
