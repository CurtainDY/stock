package main

import (
	"context"
	"flag"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"

	"github.com/parsedong/stock/backtest-engine/internal/data"
	pb "github.com/parsedong/stock/backtest-engine/proto"
)

var (
	port    = flag.String("port", "50051", "gRPC服务端口")
	dataDir = flag.String("data", "../../data/normalized", "标准化数据目录")
)

type server struct {
	pb.UnimplementedBacktestEngineServer
	store data.DataStore
}

func (s *server) RunBacktest(_ context.Context, req *pb.BacktestRequest) (*pb.BacktestResult, error) {
	// Phase 2: Python策略通过StreamBars+SubmitOrder驱动，此处返回空结果占位
	log.Printf("RunBacktest: symbols=%v start=%s end=%s capital=%.0f",
		req.Symbols, req.StartDate, req.EndDate, req.InitCapital)
	return &pb.BacktestResult{}, nil
}

func (s *server) StreamBars(req *pb.StreamRequest, stream pb.BacktestEngine_StreamBarsServer) error {
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return err
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return err
	}

	days, err := s.store.TradingDays(start, end)
	if err != nil {
		return err
	}

	for _, day := range days {
		bars, err := s.store.LoadBarsByDate(day, req.Symbols)
		if err != nil {
			return err
		}
		for _, bar := range bars {
			if err := stream.Send(&pb.Bar{
				Symbol:    bar.Symbol,
				DateUnix:  bar.Date.Unix(),
				Open:      bar.Open,
				High:      bar.High,
				Low:       bar.Low,
				Close:     bar.Close,
				Volume:    bar.Volume,
				Amount:    bar.Amount,
				AdjFactor: bar.AdjFactor,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *server) SubmitOrder(_ context.Context, _ *pb.Order) (*pb.Fill, error) {
	// Phase 2实现：撮合逻辑将在此处集成
	return &pb.Fill{Status: pb.OrderStatus_REJECTED, RejectReason: "not implemented in phase 1"}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterBacktestEngineServer(s, &server{store: data.NewParquetStore(*dataDir)})
	log.Printf("backtest engine listening on :%s", *port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
