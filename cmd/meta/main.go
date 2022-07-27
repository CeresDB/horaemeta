// Copyright 2022 CeresDB Project Authors. Licensed under Apache-2.0.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/CeresDB/ceresmeta/pkg/coderr"
	"github.com/CeresDB/ceresmeta/pkg/log"
	"github.com/CeresDB/ceresmeta/server"
	"github.com/CeresDB/ceresmeta/server/config"
	"go.uber.org/zap"
)

func panicf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	panic(msg)
}

func main() {
	cfgParser, err := config.MakeConfigParser()
	if err != nil {
		panicf("fail to generate config builder, err:%v", err)
	}

	cfg, err := cfgParser.Parse(os.Args[1:])
	if coderr.Is(err, coderr.PrintHelpUsage) {
		return
	}

	if err != nil {
		panicf("fail to parse config from command line params, err:%v", err)
	}

	if err := cfg.ValidateAndAdjust(); err != nil {
		panicf("invalid config, err:%v", err)
	}

	logger, err := log.InitGlobalLogger(&cfg.Log)
	if err != nil {
		panicf("fail to init global logger, err:%v", err)
	}
	defer logger.Sync() //nolint:errcheck

	// TODO: Do adjustment to config for preparing joining existing cluster.

	srv, err := server.CreateServer(cfg)
	if err != nil {
		log.Error("fail to create server", zap.Error(err))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	var sig os.Signal
	go func() {
		sig = <-sc
		cancel()
	}()

	if err := srv.Run(ctx); err != nil {
		log.Error("fail to run server", zap.Error(err))
		return
	}

	<-ctx.Done()
	log.Info("got signal to exit", zap.Any("signal", sig))

	srv.Close()
}
