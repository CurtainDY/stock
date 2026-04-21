package svc

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/parsedong/stock/api-server/internal/config"
	pb "github.com/parsedong/stock/api-server/proto"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
	Engine pb.BacktestEngineClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := sql.Open("postgres", c.Database.DSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Printf("WARNING: db ping failed: %v (running without DB)", err)
	}

	addr := fmt.Sprintf("%s:%d", c.BacktestEngine.Host, c.BacktestEngine.Port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("WARNING: grpc dial failed: %v (running without engine)", err)
	}

	var engineClient pb.BacktestEngineClient
	if conn != nil {
		engineClient = pb.NewBacktestEngineClient(conn)
	}

	return &ServiceContext{
		Config: c,
		DB:     db,
		Engine: engineClient,
	}
}
