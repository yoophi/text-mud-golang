package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yoophi/text-mud-golang/internal/adapter/sqlite"
	"github.com/yoophi/text-mud-golang/internal/adapter/tcp"
	"github.com/yoophi/text-mud-golang/internal/adapter/worldfile"
	"github.com/yoophi/text-mud-golang/internal/application"
	"github.com/yoophi/text-mud-golang/internal/domain"
)

func main() {
	addr := flag.String("addr", ":4000", "TCP listen address")
	dbPath := flag.String("db", "data/mud.db", "SQLite database path")
	worldPath := flag.String("world", "configs/world.json", "world definition file")
	operators := flag.String("operators", "", "comma-separated operator character names")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	world, err := worldfile.Load(*worldPath)
	if err != nil {
		logger.Error("월드 정의를 불러올 수 없습니다", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if dir := filepath.Dir(*dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			logger.Error("데이터 디렉터리 생성 실패", "err", err)
			os.Exit(1)
		}
	}
	repo, err := sqlite.Open(ctx, *dbPath)
	if err != nil {
		logger.Error("데이터베이스 열기 실패", "err", err)
		os.Exit(1)
	}
	defer repo.Close()

	game := domain.NewGame(world, application.SystemClock{}, application.SystemRandom{})

	inputs := make(chan application.Input, 256)
	gateway := tcp.NewGateway(logger, inputs)
	bound, err := gateway.Listen(*addr)
	if err != nil {
		logger.Error("리스닝 실패", "err", err)
		os.Exit(1)
	}
	logger.Info("MUD 서버 시작", "addr", bound, "rooms", len(world.Rooms()))

	var operatorNames []string
	for _, name := range strings.Split(*operators, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			operatorNames = append(operatorNames, trimmed)
		}
	}

	server := application.NewServer(application.Config{Operators: operatorNames}, game, repo, gateway, inputs, logger)
	if err := server.Run(ctx); err != nil {
		logger.Error("서버 종료 중 오류", "err", err)
		os.Exit(1)
	}
	logger.Info("서버가 종료되었습니다")
}
