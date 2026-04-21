package main

import (
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"

	"github.com/parsedong/stock/api-server/internal/config"
	"github.com/parsedong/stock/api-server/internal/handler"
	"github.com/parsedong/stock/api-server/internal/svc"
)

var configFile = flag.String("f", "etc/api-server.yaml", "config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf,
		rest.WithCors("*"),
	)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting api-server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
