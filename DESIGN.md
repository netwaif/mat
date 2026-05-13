# mat — MVP Design

## 목적

[multi-agent-starter](https://github.com/netwaif/multi-agent-starter) 시스템의
한 작업 진행 상황을 터미널에서 간결하게 모니터링한다.

MVP는 **모니터링만**. 작업 생성/승인/호출은 범위 밖.

## MVP 기능

### 포함

| 화면 요소 | 데이터 소스 | 방법 |
|---------|-----------|------|
| 작업 status / goal | `tasks/<task>/task.md` | YAML 헤더 파싱 |
| 워커 목록 | `tasks/<task>/workers/*/` 스캔 | 디렉토리 ls |
| 각 워커 상태 (대기·실행 중·완료·에러) | brief.md / result.md / log.md | 파일 stat + 태그 파싱 |
| 각 워커가 무엇을 하는지 | brief.md 첫 줄 또는 `purpose` 필드 | 텍스트 추출 |
| result.md 경로·크기 | `workers/<role>/result.md` | 파일 stat |
| log.md 마지막 5줄 | `tasks/<task>/log.md` | tail 5 |
| 작업 전환 | `tasks/` 폴더 스캔 | 디렉토리 ls |
| 자동 새로고침 | 2초 주기 폴링 | `tea.Tick` + 파일 다시 읽기 |
| 즉시 새로고침 | `r` 키 | 폴링 안 기다리고 바로 재로드 |
| 종료 | `q` 키 | exit |

### 제외 (불완전·버그 소지)

- 실시간 파일 watch (fsnotify) — OS별 동작 차이, 에디터 임시파일 노이즈 (대안으로 폴링 채택)
- log.md 실시간 tail 스트림 — 파일 watch 의존
- 산출물 폴더 실시간 변화 추적 — 같은 이유
- result.md 마크다운 렌더링 — TUI에서 까다로움. MVP는 경로·크기만
- 작업 생성·승인·워커 호출 — MVP는 모니터링만
- Discord/푸시 알림 — 별도 단계
- 다중 작업 동시 모니터링 — 한 작업 집중. 작업 전환만 지원

## 상태 아이콘

| 아이콘 | 의미 | 판정 기준 |
|--------|------|---------|
| `[ ✓ ]` | 완료 | `result.md` 존재 + 비어있지 않음 |
| `[ ⏳ ]` | 실행 중 | `brief.md` 존재 + `result.md` 없음(또는 빈 파일) |
| `[ · ]` | 대기 | `brief.md` 없음 (task.md의 planned_workers엔 명시) |
| `[ ✗ ]` | 에러 | log.md의 해당 워커 마지막 태그가 `[ERROR]` |

## 와이어프레임

```
┌──────────────────────────────────────────────────────────┐
│ mat — Task: mat-mvp                                      │
│ Status: waiting_codex-main   ·   Updated: 14:35          │
├──────────────────────────────────────────────────────────┤
│ Goal                                                     │
│   mat TUI 도구 MVP 만들기                                │
├──────────────────────────────────────────────────────────┤
│ Workers                                                  │
│                                                          │
│  [ ✓ ] claude-main                완료    14:30          │
│        설계 문서 작성                                    │
│        result.md: workers/claude-main/result.md (3.2KB)  │
│                                                          │
│  [ ⏳ ] codex-main                실행 중                 │
│        Go TUI 코드 작성 (target: ~/VSCodeWorkspace/mat)  │
│        brief.md: workers/codex-main/brief.md (980자)     │
│                                                          │
│  [ · ] codex-critic               대기                   │
│        실행 예정 (claude-main 산출물 검토)               │
│                                                          │
├──────────────────────────────────────────────────────────┤
│ Recent log (last 5)                                      │
│  [14:30] [VERIFICATION] claude-main 통과                 │
│  [14:31] [APPROVAL] codex-main 사용자 승인              │
│  [14:33] [APPROVAL] 외부 쓰기 승인. target=~/.../mat    │
│  [14:34] [WORKER_CALL] codex-main 호출. brief=980자     │
│  [14:35] [DECISION] image_gen 도구 사용                 │
├──────────────────────────────────────────────────────────┤
│ 자동 갱신 2s · [r] 즉시   [t] 작업 전환   [q] 종료       │
└──────────────────────────────────────────────────────────┘
```

## 작업 전환 (`t` 키) 동작

`t` 키를 누르면 `tasks/` 폴더의 작업 목록 모달.

```
┌─── Tasks (select with j/k, enter to open) ────────┐
│  → mat-mvp                  waiting_codex-main    │
│    blog-cards-mvp           done                  │
│    20260513-receipt-fix     in_progress           │
│                                                   │
│  [esc] 취소                                       │
└───────────────────────────────────────────────────┘
```

`enter`로 선택, `esc`로 취소.

## 기술 스택

- **언어**: Go
- **TUI**: [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss)
- **배포**: 단일 바이너리 + Homebrew tap (`netwaif/homebrew-tap`)

## 입력·출력 계약

- **입력**: 환경변수 `MAT_ROOT` 또는 cwd가 `~/VSCodeWorkspace/MultiAgent` 같은 starter root
- **활성 작업 결정 순서**:
  1. 커맨드 인자 `mat <task-name>` 명시 시 그 작업
  2. 없으면 `task.md`의 status가 `in_progress` 또는 `waiting_*`인 작업 중 가장 최근 수정
  3. 없으면 작업 목록 모달부터 띄움

## 폴더 구조 (구현 시)

```
mat/
├── main.go               # 엔트리포인트
├── go.mod
├── internal/
│   ├── model/            # task/worker/log 데이터 모델
│   ├── parser/           # task.md, brief.md, log.md 파싱
│   └── ui/               # bubbletea View/Update
├── README.md
├── DESIGN.md             # 이 문서
└── .gitignore
```
