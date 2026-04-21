package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	BacktestEngine struct {
		Host string
		Port int
	}

	Database struct {
		DSN string
	}

	Python struct {
		Executable string
		WorkDir    string
	}
}
