import uuid
import grpc
from backtest.proto import backtest_pb2_grpc


class BacktestClient:
    """gRPC 连接封装，自动管理 channel 生命周期。"""

    def __init__(self, host: str = "localhost", port: int = 50051):
        self._channel = grpc.insecure_channel(f"{host}:{port}")
        self.stub = backtest_pb2_grpc.BacktestEngineStub(self._channel)

    def new_session_id(self) -> str:
        return str(uuid.uuid4())

    def close(self) -> None:
        self._channel.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
