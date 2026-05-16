# mat 파서/상태 판정 수정 — 설계 (2026-05-16)

## 배경

mat은 MultiAgent 시스템(`~/VSCodeWorkspace/MultiAgent`)을 **읽기 전용**으로 모니터링하는
Go TUI다. mat 파서가 가정하는 파일 포맷을 실제 시스템(`_templates/`, `mat-log-improve`·
`hwpx-math-final-v3` 등 실제 task 폴더)과 대조한 결과, 시스템이 실제로 만들어내는 파일을
mat이 잘못 읽는 결함 3건을 확정했다. codex(gpt-5.5 high)가 코드를 직접 읽어 교차검증했다.

이 작업은 **mat repo만** 수정한다. `~/VSCodeWorkspace/MultiAgent/` 아래는 한 글자도 쓰지
않는다(mat은 설계상 모니터, MultiAgent는 읽기 전용).

## 문제 (확정된 결함)

- **#1 — 수정 반복 파일이 안 보임 (+M1 흡수).** 워커는 `result.md` → `result-fix.md` →
  `result-fix2.md`(및 `brief-fix*.md`)로 반복한다. `bestResultFile`는 `result.`(점) 패턴만
  잡아 `result-fix.md`(하이픈)를 무시한다. fix 2회 후에도 mat은 원본 `result.md`의
  크기·경로를 보이고 워커 `UpdatedAt`이 낡는다 — "지금 무슨 일이 일어나는가"가 틀린다.
  (검증: `task.go:225-263`; 실제 로그 `mat-log-improve/log.md:11,:23,:31`.)
- **#2 — `reviewing` 상태 무시.** `_templates/task.md`의 상태 집합은 `pending |
  in_progress | waiting_<role> | reviewing | done`. `reviewing`은 "사용자 확인 단계"로
  정확히 mat을 켤 시점인데 `PickActiveTask`는 `in_progress`/`waiting_*`만 자동 선택하고,
  `statusStyle`에 `reviewing`·`pending` 색상이 없다(muted 회색).
  (검증: `task.go:334-375`, `view.go:40-53`.)
- **#3 — 실패/초안 결과가 완료로 표시.** `_templates/worker-result.md`에 yaml
  `status: draft | complete | failed`가 있는데 `buildWorker`는 비어있지 않은 result 파일이면
  무조건 `StateDone`으로 본다. `failed` 워커가 `[ ✓ ] 완료`로 보인다.
  (검증: `worker-result.md:5-11`, `task.go:202-216`.)

### 범위 밖 (별도 task로 분리, 본 설계에 미포함)

- **M2** `workers_approved` 미표시 — 실제 누락이나 *기능 추가*(승인 게이트 가시화)이지
  정확성 버그 아님. model+parser+view 확장이라 별도.
- **M3** 로그 TAG 미인식/색상화 — DESIGN.md 와이어프레임에도 없는 *향상*.
  `workerHasError` substring 매칭은 role 이름이 full이라 실무 오탐 위험 낮음.
- **M1(원안)** "최신 mtime 우선" — `result.tokens.json`은 실제 산출물이나 `result.md`가
  정본(yaml status·summary·completed_at 보유). 원안대로 "정본 무시하고 최신 우선"은
  기존 테스트/설계 회귀. M1의 타당한 핵심(시간 신선도)은 #1에 흡수한다.
- **#4** task-level `artifacts/` 노출 — DESIGN.md가 MVP에서 명시 제외한 항목.

## 변경 파일 (격리 범위)

| 경로 | 변경 |
|---|---|
| `internal/parser/task.go` | `buildWorker` 재작성, result 폴백 보강, `PickActiveTask` 술어 1줄 |
| `internal/ui/view.go` | `statusStyle`에 `reviewing`/`pending` case + `colorReview` |
| `internal/parser/task_test.go` | `buildWorker` 테이블 확장 + `PickActiveTask` 테스트 신규 |
| `DESIGN.md` | 상태판정·활성작업·반복파일 규칙 패치 (CLAUDE.md상 spec of record) |

**무변경**: `internal/model/types.go`(상태 enum 4종 재사용), `internal/ui/model.go`
(폴링·모달·레이아웃), `main.go`. 외부 의존성 0 추가.

## 설계

### 1. 워커 파일 분류 (`buildWorker`)

한 워커 디렉터리(`workers/<role>/`) 안의 파일을 분류한다:

- **brief 파일** — `strings.HasPrefix(name,"brief") && strings.HasSuffix(name,".md")`
  → `brief.md`, `brief-fix.md`, `brief-fix2.md`
- **result md 파일** — `strings.HasPrefix(name,"result") && strings.HasSuffix(name,".md")`
  → `result.md`, `result-fix.md`, `result-fix2.md`, `result.foo.md`
- **result 아티팩트** — 비-`.md` result(`result.tokens.json`, `result.output.json`,
  generic `result.*`), 단 `result.partial-*` **제외**
- **partial** — `result.partial-*` — UpdatedAt 신선도에만 사용, Done/표시엔 미사용

**반복 번호 `revision(name)`** — 확장자를 제거한 이름이 `-fix`로 끝나면 1, `-fix<N>`이면
`<N>`, 그 외 0. 정규식 `-fix(\d*)$`(확장자 제거 후), 매치 시 숫자 없으면 1. 예:
`result.md`→0, `result-fix.md`→1, `result-fix2.md`→2, `result.foo.md`→0, `brief-fix.md`→1.
mtime은 git 체크아웃·복사로 동일/역전될 수 있어 신뢰 불가하므로, "어느 것이 최신
반복인가"의 **권위 신호는 반복 번호**다.

선택 규칙 — 비어있지 않은 후보 중 `(revision, mtime, name)` 사전식 최대값:

- `briefSel` = brief 파일 중 `(revision DESC, mtime DESC, name DESC)` 최대.
  `HasBrief`/`BriefPath`/`BriefSize`/`BriefChars`/`Purpose`를 여기서 채운다.
  `Purpose`는 `firstMeaningfulLine(briefSel)`, 비면 planned purpose 폴백(기존 로직 유지).
- `resultSel` = result md 파일 중 `(revision DESC, mtime DESC, name DESC)` 최대.
  result md가 하나도 없으면 아티팩트 폴백: `result.tokens.json` → `result.output.json`
  → 정렬된 generic non-partial 중 첫 비어있지 않은 것(기존 `bestResultFile` 우선순위 보존).
  `HasResult`/`ResultPath`/`ResultSize`를 여기서 채운다.
- `UpdatedAt` = 이름이 `brief` 또는 `result`로 시작하는 워커 폴더 내 **모든 파일**
  (partial 포함)의 최신 mtime. (UpdatedAt은 "마지막 활동 시각" 개념이라 mtime 기준 유지.)

### 2. result yaml status 파싱 (#3)

`resultSel`이 `.md` 파일이면 기존 `readYAMLHeader(resultSel)`로 첫 ```yaml 블록의
`status` 키를 읽는다(trim 후 소문자 비교). worker-result.md의 첫 yaml 블록이 메타
블록이라 대상이 맞다.

- `failed` → resultStatus = failed
- `draft` → resultStatus = draft
- 그 외 / 키 없음 / 파싱 실패 / resultSel이 JSON 아티팩트 → resultStatus = normal

### 3. 워커 상태 머신 (#1·#3 통합, enum 재사용)

우선순위 高→低 (기존 `model.State{Error,Running,Done,Pending}` 재사용, 신규 enum 없음):

1. `workerHasError(role, logPath)` (log `[ERROR]`, **기존 그대로**) → `StateError`
2. 비어있지 않은 result 있음(`HasResult`):
   - resultStatus `failed` → `StateError`
   - resultStatus `draft` → `StateRunning`
   - 그 외 → `StateDone`
3. `HasBrief` → `StateRunning`
4. 그 외 → `StatePending`

### 4. 활성 작업 + 스타일 (#2)

- `PickActiveTask` 술어:
  `status == "in_progress" || status == "reviewing" || strings.HasPrefix(status,"waiting_")`.
  최신 task.md mtime 동률처리는 그대로.
- `view.go`: `colorReview = lipgloss.Color("213")`(마젠타) 추가.
  `statusStyle`에 `case s == "reviewing":` → `colorReview`(Bold),
  `case s == "pending":` → `colorMuted`(명시화). 나머지 case 불변.

### 5. 데이터 흐름

기존 파이프라인 불변: `LoadTask` → 워커 디렉터리별 `buildWorker` → `model.Task` → UI.
`buildWorker` 내부 파일선택·상태로직, `PickActiveTask` 술어, `statusStyle` case만 바뀐다.
폴링(2s tick)·모달·레이아웃·overlay·truncate는 손대지 않는다.

## 엣지 케이스

- partial-only(gemini 스트리밍 중): `HasResult=false`(partial 제외) → `StateRunning`
  유지(brief 있으면). `UpdatedAt`은 partial mtime 반영 → 시간만 신선해짐(개선).
- `result.md` 비었고 `result-fix.md` 참 → 최신 비어있지 않은 result md = `result-fix.md`
  → `StateDone`(엣지 개선).
- result가 JSON 아티팩트뿐(`result.tokens.json`만) → yaml 없음 → resultStatus normal
  → `StateDone`(기존 호환).
- yaml `status` 값의 대소문자·공백 → trim + ToLower 후 비교.
- 워커 디렉터리 빈 폴더 → brief/result 없음 → `StatePending`(planned면 그 purpose).
- 같은 반복 번호 파일 다수(예 `result.foo.md`, `result.bar.md` 둘 다 rev0) → mtime,
  그래도 동률이면 name으로 결정론적 처리(실제 데이터엔 없으나 정의는 둠).
- 비-`.md` 반복 아티팩트(`result-fix.tokens.json` 등, 실제 미관측) → result md 아님 →
  아티팩트 폴백의 고정 우선순위로 처리(revision 미적용).

## 테스트

**`task_test.go` 기존 7케이스 무변경 통과** (검증 완료):
result.md가 유일 `.md`라 "최신 .md"=result.md / tokens-only는 폴백 / partial-only는 미완료
/ empty result.md는 미완료 / generic `result.foo.md`는 .md라 선택됨.

**신규 `buildWorker` 케이스:**

- brief.md+result.md+result-fix.md+result-fix2.md → `StateDone`,
  `ResultPath`=result-fix2.md (revision 2가 권위 신호라 mtime 조작 불필요·결정론적)
- result-fix.md만(result.md 없음) → `StateDone`, `ResultPath`=result-fix.md
- brief.md+brief-fix2.md(fix가 더 최신) → `BriefPath`=brief-fix2.md, `BriefChars` 그 값
- result.md yaml `status: failed` → `StateError`
- result.md yaml `status: draft` → `StateRunning`
- result.md yaml `status: complete` → `StateDone`
- result.md yaml 없음 → `StateDone`(기존 호환)
- result.partial-*.json만 → `UpdatedAt` 신선, 상태 `StateRunning`(brief 있을 때)

**신규 `PickActiveTask` 테스트:** temp task 디렉터리에 pending/in_progress/reviewing/
waiting_x/done 혼합 → 반환값이 {in_progress, reviewing, waiting_*} 중 최신 task.md mtime.

**수동 검증:** `go build ./... && go vet ./... && go test ./...` 전부 green,
`MAT_ROOT=~/VSCodeWorkspace/MultiAgent ./mat mat-log-improve`로 워커 시간이 fix2 반영됨
육안 확인.

## DESIGN.md 패치 항목

- 상태 아이콘 표: `[ ✗ ]` 에러 판정 = "log.md 마지막 [ERROR] **또는** 선택된 result*.md
  yaml `status: failed`". `[ ⏳ ]` 실행 중에 "result*.md yaml `status: draft`" 추가.
- 워커 파일: brief/result는 `*-fix*` 반복 파일 중 **최신 mtime**을 표시하고,
  `UpdatedAt`은 워커 폴더의 모든 brief*/result*(partial 포함) 최신 mtime 한 줄 추가.
- "활성 작업 결정 순서"에 `reviewing` 포함.
- 와이어프레임·기존 절은 유지.
