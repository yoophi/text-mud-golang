package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/yoophi/text-mud-golang/internal/domain"
)

// Config holds application-level settings.
type Config struct {
	Operators    []string
	TickInterval time.Duration
}

// shutdownTimeout bounds the graceful shutdown sequence.
const shutdownTimeout = 5 * time.Second

// Server is the single game loop. It consumes Input events, drives the
// domain engine, and applies the resulting Effects through outbound
// ports. All game state changes happen on this goroutine.
type Server struct {
	cfg    Config
	game   *domain.Game
	repo   CharacterRepository
	net    NetGateway
	inputs <-chan Input
	log    *slog.Logger

	logins map[domain.SessionID]*loginFlow

	shutdown       bool
	shutdownReason string
}

// loginFlow tracks a session that has not entered the world yet.
type loginFlow struct {
	stage int
	name  string
}

const (
	loginAskName = iota
	loginConfirmCreate
)

var namePattern = regexp.MustCompile(`^[가-힣A-Za-z0-9]{2,12}$`)

// NewServer wires the loop together. inputs must be the same channel the
// network gateway reports events on.
func NewServer(cfg Config, game *domain.Game, repo CharacterRepository, net NetGateway, inputs <-chan Input, log *slog.Logger) *Server {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 100 * time.Millisecond
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:    cfg,
		game:   game,
		repo:   repo,
		net:    net,
		inputs: inputs,
		log:    log,
		logins: map[domain.SessionID]*loginFlow{},
	}
}

// Run enters the event loop and returns after a graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return s.gracefulShutdown("종료 신호 수신")
		case in, ok := <-s.inputs:
			if !ok {
				return s.gracefulShutdown("입력 채널이 닫혔습니다")
			}
			s.handleInput(ctx, in)
			if s.shutdown {
				return s.gracefulShutdown(s.shutdownReason)
			}
		case now := <-ticker.C:
			s.apply(s.game.Tick(now))
			if s.shutdown {
				return s.gracefulShutdown(s.shutdownReason)
			}
		}
	}
}

// handleInput routes one inbound event.
func (s *Server) handleInput(ctx context.Context, in Input) {
	switch in.Kind {
	case InputConnected:
		s.logins[in.Session] = &loginFlow{stage: loginAskName}
		s.net.Write(in.Session, "텍스트 MUD 서버에 오신 것을 환영합니다.\n이름을 입력하세요:")
	case InputLine:
		if s.game.Playing(in.Session) {
			s.apply(s.game.ExecuteLine(in.Session, in.Line))
			return
		}
		s.loginStep(ctx, in.Session, in.Line)
	case InputDisconnected:
		delete(s.logins, in.Session)
		s.apply(s.game.Disconnect(in.Session))
	}
}

// loginStep runs the character creation / login flow.
func (s *Server) loginStep(ctx context.Context, session domain.SessionID, line string) {
	flow, ok := s.logins[session]
	if !ok {
		s.logins[session] = &loginFlow{stage: loginAskName}
		flow = s.logins[session]
	}
	fields := strings.Fields(domain.Normalize(line))
	if len(fields) == 0 {
		s.net.Write(session, "이름을 입력하세요:")
		return
	}
	word := fields[0]

	switch flow.stage {
	case loginAskName:
		name := word
		if !namePattern.MatchString(name) {
			s.net.Write(session, "이름은 한글·영문·숫자 2~12자로 입력하세요.\n이름을 입력하세요:")
			return
		}
		if s.game.IsOnline(name) {
			s.net.Write(session, "이미 접속 중인 이름입니다. 다른 이름을 입력하세요:")
			return
		}
		char, err := s.repo.Load(ctx, name)
		if err != nil && !errors.Is(err, ErrCharacterNotFound) {
			s.log.Error("캐릭터 조회 실패", "name", name, "err", err)
			s.net.Write(session, "캐릭터를 조회하는 데 실패했습니다. 잠시 후 다시 시도하세요.\n이름을 입력하세요:")
			return
		}
		if err == nil {
			s.enterWorld(session, char)
			return
		}
		flow.name = name
		flow.stage = loginConfirmCreate
		s.net.Write(session, fmt.Sprintf("'%s'라는 새 캐릭터를 생성할까요? (네/아니오)", name))

	case loginConfirmCreate:
		switch strings.ToLower(word) {
		case "네", "예", "y", "yes":
			char := domain.NewCharacter(flow.name, s.game.World().Start())
			delete(s.logins, session)
			s.enterWorld(session, char)
		case "아니오", "아니요", "n", "no":
			flow.stage = loginAskName
			s.net.Write(session, "이름을 입력하세요:")
		default:
			s.net.Write(session, "'네' 또는 '아니오'를 입력하세요.")
		}
	}
}

// enterWorld connects the character and applies the engine effects.
func (s *Server) enterWorld(session domain.SessionID, char *domain.Character) {
	for _, op := range s.cfg.Operators {
		if op == char.Name {
			char.Operator = true
			break
		}
	}
	s.apply(s.game.Connect(session, char))
}

// apply routes engine effects to the outbound ports.
func (s *Server) apply(effects []domain.Effect) {
	for _, e := range effects {
		switch e := e.(type) {
		case domain.Output:
			s.net.Write(e.Session, e.Text)
		case domain.Broadcast:
			for _, target := range e.Sessions {
				s.net.Write(target, e.Text)
			}
		case domain.SaveCharacter:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := s.repo.Save(ctx, e.Character); err != nil {
				s.log.Error("캐릭터 저장 실패", "name", e.Character.Name, "err", err)
			}
			cancel()
		case domain.Shutdown:
			if !s.shutdown {
				s.shutdown = true
				s.shutdownReason = e.Reason
			}
		}
	}
}

// gracefulShutdown disconnects everyone (which saves state), stops the
// network, and reports errors. It never hangs longer than the timeout.
func (s *Server) gracefulShutdown(reason string) error {
	s.log.Info("서버를 종료합니다", "reason", reason)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, session := range s.game.Sessions() {
			s.apply(s.game.Disconnect(session))
		}
		s.net.Stop()
	}()
	select {
	case <-done:
		return nil
	case <-time.After(shutdownTimeout):
		err := fmt.Errorf("안전한 종료가 %s 안에 완료되지 않았습니다", shutdownTimeout)
		s.log.Error("종료 타임아웃", "err", err)
		return err
	}
}
