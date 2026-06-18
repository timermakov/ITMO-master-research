# Методика бенчмаркинга

## Принцип

Каждый **run** — независимое измерение на **холодном** warmup-инстансе. Между run выполняется `POST /reset` (сброс mmap+индекса и warmkit FSM → Registered). Сценарии **не** гоняются подряд на одном горячем процессе.

## Первичная метрика

**T_first** = `duration_ns` из **первого и единственного** боевого `GET /work` (без `X-Warmup-Shadow`) после фазы прогрева в данном run.

Вторичная метрика: **E2E** — wall-clock от клиента probe до ответа.

Единый профиль во всех сценариях:

```json
{ "seed": "<int64>", "samples": "<int>" }
```

## Порядок действия при запуске

```text
POST /reset  →  [фаза прогрева по сценарию]  →  один GET /work (probe)  →  cooldown
```

| Сценарий | Фаза прогрева | Probe |
|----------|---------------|-------|
| **S0** | нет | сразу `/work` |
| **S_ref** | `POST /warmup` с тем же Profile | `/work` |
| **S_dw** | baseline mirror → coordinator session → loadgen `GET /work` через proxy → `/readyz`=200 | `/work` на warmup URL |

## S_dw (динамический)

1. `PUT /v1/mirror/config` — mirror off, active = `active-1`.
2. `POST /v1/warmup/sessions` — ramp через coordinator.
3. Параллельно: loadgen шлёт `GET /work?seed=&samples=` **через proxy** по профилю **ramp-up → steady → ramp-down** (см. ниже).
4. Ждём `session.status=completed` и `/readyz=200`.
5. Один probe на warmup (не через proxy, без shadow header).

Coordinator ramp **60 s** (`RAMP_INTERVAL=10` × 6 steps), aligned with loadgen ramp-up. После шагов — **hold** mirror at ratio=1.0 до `Ready`/`Active` (max `DWSS_COORD_HOLD_MAX_SEC`).

benchmark-bench **не пишет в ZK** — только HTTP coordinator.

По [рекомендациям по съёму метрик](https://habr.com/ru/articles/910760/): метрики **не** усредняются по всему прогону. Loadgen реализует три фазы:

| Фаза | Переменная | Назначение |
|------|------------|------------|
| **Ramp-up** | `DWSS_BENCH_RAMP_UP_SEC` | Плавный рост RPS 0→max (прогрев кэша, соединений) |
| **Steady** | `DWSS_BENCH_STEADY_SEC` | Постоянная нагрузка; **только здесь** считаются E2E/H4 метрики |
| **Ramp-down** | `DWSS_BENCH_RAMP_DOWN_SEC` | Плавное снижение RPS; в статистику **не** входит |

RPS в ramp-up/down растёт/падает **линейно**. Для S_dw steady нужен для прогрева shadow через proxy; **T_first** по-прежнему снимается одним probe после завершения всей фазы нагрузки.

Thesis-профиль: ramp **60 s**, steady **≥300 s** (5 min — минимум для устойчивых метрик), down **30 s**. H4 использует тот же ramp/down, но `DWSS_BENCH_H4_STEADY_SEC` для steady-окна E2E.

## H4 (overhead mirror)

Измеряется **E2E p95** `GET /work` **через proxy** (клиентский путь на active):

1. `mirror off` → loadgen (ramp → steady → down) → RTT **только из steady**.
2. `mirror on, ratio=1.0` → loadgen → RTT из steady.
3. Сравнить p95 block-medians: H4 pass если p95(on) ≤ p95(off) × 1.05.

После H4 — baseline mirror off.

## Статистика

- `DWSS_BENCH_RUNS ≥ 5` (рекомендуется **10**); bench завершится с ошибкой при меньшем значении.
- `DWSS_BENCH_READY_AFTER` **должен совпадать** с `DWSS_WARMUP_READY_AFTER` (проверка через `GET /state` при старте).
- **Valid run**: перед probe проверяется `GET /state` (cold для S0, warm для S_ref/S_dw); invalid run повторяется, в stats не попадает.
- После `POST /warmup` (S_ref) — poll `/state` до 2 s, пока `appCold=false`.
- Агрегация по **P50/P95 workload_ns** по valid runs; **95% bootstrap CI** для mean и **P50**.
- **CV** ≤ `DWSS_BENCH_CV_THRESHOLD` (thesis ideal **5%**; lab stand Windows/Docker **15–20%**; см. `.env.local`)
- Выбросы: фильтр **1.5×IQR**; индексы в `outlierRunIndexes`; raw runs сохраняются в JSON.
- **Adaptive runs** (`DWSS_BENCH_PROFILE_ON_HIGH_CV=true`): при CV fail после `RUNS` — до `DWSS_BENCH_MAX_RUNS`; иначе exit 1.

### Профили конфигурации

| Профиль | RUNS | RAMP / STEADY / DOWN | COOLDOWN_MS | Назначение |
|---------|------|----------------------|-------------|------------|
| **Thesis** | 10 | 60 / 300 / 30 | 1000 | финальная серия для ВКР |
| **Debug** | 5 | 30 / 60 / 15 | 500 | быстрая проверка стенда |

## H4 — агрегация блоков

RTT агрегируются в **1-секундные блоки** (размер блока = `RPS`): медиана RTT в блоке. Блоки строятся **только из steady-фазы** (≈120 точек при H4 steady=120 s). P95 и CV считаются по block-medians.

## Изоляция и порядок

- Сценарии запускать **отдельно** или одной командой `bench run all` (внутри — полный reset между каждым run каждого сценария).
- `DWSS_BENCH_COOLDOWN_MS` — пауза между reset и probe / между runs.
- Не смешивать profiling (pprof) с замерами T_first.

## Manifest (`results.json`)

- `protocolVersion`, `gitCommit`, `GOMAXPROCS`, `startedAt`
- URLs, Profile, runs, `runs[]` с per-run probe + warmkit state
- `hypotheses`: H1, H2, H2_CI, H3 (для `bench run all`), H4 (для `bench h4`)

## Гипотезы

| ID | Формулировка (по median P50 T_first) |
|----|--------------------------------------|
| H1 | S_dw ≤ S0 / 2 |
| H2 | S_dw ≤ S_ref × 1.1 (P50) |
| H2_CI | upper bound P50 bootstrap CI для S_dw ≤ S_ref × 1.1 |
| H3 | `/readyz` = 200 после S_dw (логируется в `runs[].readyOk`) |
| H4 | p95 E2E proxy `/work`: mirror on ≤ off × 1.05 |

## CLI

```bash
make bench-s0
make bench-s-ref
make bench-s-dw
make bench-h4
make bench-all    # S0 + S_ref + S_dw + hypotheses
```

```bash
cd benchmark-bench
go run ./cmd/bench run s0-control --out ../results/s0
go run ./cmd/bench run all --out ../results/full
```

## Profiling

Отдельный прогон при CV > порога:

```bash
make profile-flame   # http://127.0.0.1:6060 — warmup pprof
```

См. [Flame Graphs for Go With pprof](https://www.benburwell.com/posts/flame-graphs-for-go-with-pprof/).

## Воспроизводимость

Перед серией прогонов: зафиксировать docker image ids, `.env.local`, commit. Результаты — в `Projects/results/` (gitignored).
