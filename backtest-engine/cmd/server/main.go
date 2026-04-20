package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/parsedong/stock/backtest-engine/internal/data"
	"github.com/parsedong/stock/backtest-engine/internal/matcher"
	"github.com/parsedong/stock/backtest-engine/internal/portfolio"
	pb "github.com/parsedong/stock/backtest-engine/proto"
)

var (
	port    = flag.String("port", "50051", "gRPC服务端口")
	dataDir = flag.String("data", "../../data/normalized", "标准化数据目录")
)

type session struct {
	portfolio   *portfolio.Portfolio
	matcher     *matcher.Matcher
	currentBars map[string]data.Bar
	prevPrices  map[string]float64
	mu          sync.Mutex
}

type server struct {
	pb.UnimplementedBacktestEngineServer
	store    data.DataStore
	sessions sync.Map // string → *session
}

func sessionIDFromCtx(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("x-session-id")
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func dateFromCtx(ctx context.Context) time.Time {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return time.Time{}
	}
	vals := md.Get("x-date-unix")
	if len(vals) == 0 {
		return time.Time{}
	}
	var unix int64
	if _, err := fmt.Sscanf(vals[0], "%d", &unix); err != nil {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}

func (s *server) RunBacktest(_ context.Context, req *pb.BacktestRequest) (*pb.BacktestResult, error) {
	log.Printf("RunBacktest: symbols=%v start=%s end=%s capital=%.0f",
		req.Symbols, req.StartDate, req.EndDate, req.InitCapital)
	return &pb.BacktestResult{}, nil
}

func (s *server) StreamBars(req *pb.StreamRequest, stream pb.BacktestEngine_StreamBarsServer) error {
	sessionID := sessionIDFromCtx(stream.Context())
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "x-session-id metadata required")
	}

	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid start_date: %v", err)
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid end_date: %v", err)
	}

	const defaultCapital = 1_000_000.0
	const defaultCommission = 0.0003

	sess := &session{
		portfolio:   portfolio.New(defaultCapital),
		matcher:     matcher.New(matcher.Config{Commission: defaultCommission}),
		currentBars: make(map[string]data.Bar),
		prevPrices:  make(map[string]float64),
	}

	days, err := s.store.TradingDays(start, end)
	if err != nil {
		return status.Errorf(codes.Internal, "trading days: %v", err)
	}

	s.sessions.Store(sessionID, sess)
	defer s.sessions.Delete(sessionID)

	var prevDay time.Time
	for _, day := range days {
		bars, err := s.store.LoadBarsByDate(day, req.Symbols)
		if err != nil {
			return status.Errorf(codes.Internal, "load bars %s: %v", day.Format("2006-01-02"), err)
		}

		sess.mu.Lock()
		if !prevDay.IsZero() {
			for sym, bar := range sess.currentBars {
				sess.prevPrices[sym] = bar.Close
			}
		}
		sess.currentBars = make(map[string]data.Bar, len(bars))
		for sym, bar := range bars {
			sess.currentBars[sym] = bar
		}
		sess.mu.Unlock()
		prevDay = day

		// Send bars outside the lock
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

func (s *server) SubmitOrder(ctx context.Context, req *pb.Order) (*pb.Fill, error) {
	sessionID := sessionIDFromCtx(ctx)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "x-session-id metadata required")
	}

	v, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "session %q not found", sessionID)
	}
	sess := v.(*session)

	orderDate := dateFromCtx(ctx)
	if orderDate.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "x-date-unix metadata required")
	}

	sess.mu.Lock()
	bar, hasCurrent := sess.currentBars[req.Symbol]
	prevClose := sess.prevPrices[req.Symbol]
	var availQty float64
	if req.Side == pb.OrderSide_SELL && hasCurrent {
		pos := sess.portfolio.Position(req.Symbol)
		availQty = pos.AvailableQty(orderDate)
	}
	sess.mu.Unlock()

	if !hasCurrent {
		return &pb.Fill{
			Symbol:       req.Symbol,
			Side:         req.Side,
			Status:       pb.OrderStatus_REJECTED,
			RejectReason: "no bar data for symbol on current day",
		}, nil
	}

	if req.Side == pb.OrderSide_SELL && availQty < req.Quantity {
		return &pb.Fill{
			Symbol:       req.Symbol,
			Side:         req.Side,
			Status:       pb.OrderStatus_REJECTED,
			RejectReason: fmt.Sprintf("T+1: available=%.0f, want=%.0f", availQty, req.Quantity),
		}, nil
	}

	order := matcher.Order{
		Symbol:   req.Symbol,
		Side:     matcher.Side(req.Side),
		Quantity: req.Quantity,
		Price:    req.Price,
	}
	fill := sess.matcher.Match(order, bar, prevClose, false)

	if fill.Status == matcher.Filled {
		sess.mu.Lock()
		applyErr := sess.portfolio.ApplyFill(fill, orderDate)
		sess.mu.Unlock()
		if applyErr != nil {
			return nil, status.Errorf(codes.Internal, "apply fill: %v", applyErr)
		}
	}

	// matcher.Filled=0, matcher.Rejected=1
	// pb.OrderStatus_FILLED=1, pb.OrderStatus_REJECTED=2
	var fillStatus pb.OrderStatus
	if fill.Status == matcher.Filled {
		fillStatus = pb.OrderStatus_FILLED
	} else {
		fillStatus = pb.OrderStatus_REJECTED
	}

	return &pb.Fill{
		Symbol:       fill.Symbol,
		Side:         pb.OrderSide(fill.Side),
		FilledQty:    fill.FilledQty,
		FilledPrice:  fill.FilledPrice,
		Commission:   fill.Commission,
		StampDuty:    fill.StampDuty,
		Status:       fillStatus,
		RejectReason: fill.RejectReason,
	}, nil
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
