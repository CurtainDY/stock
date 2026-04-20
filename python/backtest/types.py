from dataclasses import dataclass, field
from datetime import date
from enum import IntEnum
from typing import Optional


class OrderSide(IntEnum):
    BUY = 0
    SELL = 1


class FillStatus(IntEnum):
    PENDING = 0
    FILLED = 1
    REJECTED = 2


@dataclass
class Bar:
    symbol: str
    date: date
    open: float
    high: float
    low: float
    close: float
    volume: float
    amount: float
    adj_factor: float = 0.0

    @property
    def adj_close(self) -> float:
        """前复权价格（同花顺加法偏移）"""
        if self.adj_factor == 0:
            return self.close
        return self.close + self.adj_factor


@dataclass
class Order:
    symbol: str
    side: OrderSide
    quantity: float
    price: float = 0.0  # 0 = 市价单，以收盘价成交


@dataclass
class Fill:
    symbol: str
    side: OrderSide
    filled_qty: float
    filled_price: float
    commission: float
    stamp_duty: float
    status: FillStatus
    reject_reason: str = ""


@dataclass
class Position:
    symbol: str
    quantity: float
    cost_price: float


@dataclass
class PortfolioSnapshot:
    cash: float
    positions: dict[str, Position]  # symbol → Position
    total_value: float
