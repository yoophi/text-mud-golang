# text-mud-golang

Go로 작성하는 영속형 멀티플레이어 텍스트 MUD 서버입니다.

## 구조

헥사고날 아키텍처를 사용합니다.

- `internal/domain`: 외부 기술에 의존하지 않는 게임 모델과 규칙
- `internal/application`: 유스케이스와 입·출력 포트
- `internal/adapter`: TCP/Telnet, SQLite, 월드 파일 어댑터
- `cmd/mud-server`: 어댑터를 조립하는 실행 진입점

## 개발

Go 1.26 이상이 필요합니다.

```sh
make check
make build
make run
```
