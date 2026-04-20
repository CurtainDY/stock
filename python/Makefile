PROTO_SRC = ../backtest-engine/proto/backtest.proto
OUT_DIR   = backtest/proto

.PHONY: proto clean

proto:
	python3 -m grpc_tools.protoc \
		-I../backtest-engine/proto \
		--python_out=$(OUT_DIR) \
		--grpc_python_out=$(OUT_DIR) \
		$(PROTO_SRC)
	sed -i '' 's/^import backtest_pb2/from . import backtest_pb2/' $(OUT_DIR)/backtest_pb2_grpc.py
	@echo "Proto stubs generated in $(OUT_DIR)"

clean:
	rm -f $(OUT_DIR)/backtest_pb2*.py
