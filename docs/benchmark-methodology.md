# Методика бенчмаркинга

## Принцип

Каждый **run** — независимое измерение на **холодном** warmup-инстансе. Между run выполняется `POST /reset` (сброс in-memory hashmap и warmkit FSM → Registered). Сценарии **не** гоняются подряд на одном горячем процессе.

## Workload: index-build

Единственный workload — **построение lazy `map[int]int` при первом запросе**:

- `POST /reset` → `index = nil`, `indexCold=true`
- первый `GET /work` (или `POST /warmup`) строит hashmap (`index[i]=i`) и выполняет `samples` lookup
- **`duration_ns`** = wall-clock **build + lookup** целиком (mmap не участвует)
- целевой cold path: **10–100 ms** (`DWSS_INDEX_KEYS`, по умолчанию 100000)

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
| **S0** | нет | сразу `/work` (cold hashmap build) |
| **S_ref** | `POST /warmup` с тем же Profile | `/work` (индекс уже построен) |
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
| **Ramp-up** | `DWSS_BENCH_RAMP_UP_SEC` | Плавный рост RPS 0→max (прогрев соединений) |
| **Steady** | `DWSS_BENCH_STEADY_SEC` | Постоянная нагрузка; **только здесь** считаются E2E/overhead метрики |
| **Ramp-down** | `DWSS_BENCH_RAMP_DOWN_SEC` | Плавное снижение RPS; в статистику **не** входит |

RPS в ramp-up/down растёт/падает **линейно**. Для S_dw steady нужен для прогрева shadow через proxy; **T_first** по-прежнему снимается одним probe после завершения всей фазы нагрузки.

Thesis-профиль: ramp **60 s**, steady **≥300 s** (5 min — минимум для устойчивых метрик), down **30 s**. Сценарий **overhead** использует тот же ramp/down, но `DWSS_BENCH_H4_STEADY_SEC` для steady-окна E2E.

## Overhead (зеркалирование)

Отдельный запуск, не смешивается с T_first:

```bash
bench run --scenarios overhead --out ../results/overhead
```

Измеряется **E2E p95** `GET /work` **через proxy** (клиентский путь на active):

1. `mirror off` → loadgen (ramp → steady → down) → RTT **только из steady**.
2. `mirror on, ratio=1.0` → loadgen → RTT из steady.
3. Сравнить p95 block-medians: pass если p95(on) ≤ p95(off) × 1.05.

После overhead — baseline mirror off.

## Статистика

- `DWSS_BENCH_RUNS ≥ 20` для финального отчёта НИР; `DWSS_BENCH_MAX_RUNS=40` при adaptive runs.
- `DWSS_BENCH_PROFILE_ON_HIGH_CV=true` — при CV fail после `RUNS` добирать до `MAX_RUNS`.
- Overhead выполняется отдельными повторами `DWSS_BENCH_H4_RUNS` / `DWSS_BENCH_H4_MAX_RUNS`: каждый повтор даёт run-level P95 по 1-секундным block-medians.
- `DWSS_BENCH_READY_AFTER` **должен совпадать** с `DWSS_WARMUP_READY_AFTER` (проверка через `GET /state` при старте).
- **Valid run**: перед probe проверяется `GET /state` (`indexCold=true` для S0, `indexCold=false` для S_ref/S_dw); invalid run повторяется, в stats не попадает.
- После `POST /warmup` (S_ref) — poll `/state` до 2 s, пока `indexCold=false`.
- Агрегация по **P50/P95 workload_ns** по valid runs; **95% bootstrap CI** для mean и **P50**.
- **CV** считается по filtered set. Глобальный порог задаёт `DWSS_BENCH_CV_THRESHOLD`, но для отчётной серии допустимы scenario-specific пороги:
  - `DWSS_BENCH_CV_THRESHOLD_S0` — cold baseline может иметь более высокую естественную дисперсию.
  - `DWSS_BENCH_CV_THRESHOLD_S_REF` и `DWSS_BENCH_CV_THRESHOLD_S_DW` — прогретые сценарии, целевой **≤ 20%**.
- Цель для НИР: CV прогретых сценариев **≤ 20%**. Cold path должен быть **≥ 10 ms**, иначе scheduler jitter доминирует.
- Выбросы: фильтр **1.5×IQR**; индексы в `outlierRunIndexes`; raw runs сохраняются в JSON.
- **Adaptive runs** (`DWSS_BENCH_PROFILE_ON_HIGH_CV=true`): при CV fail после `RUNS` — до `DWSS_BENCH_MAX_RUNS`; иначе exit 1.
- `DWSS_BENCH_COOLDOWN_MS ≥ 5000` между runs.

### Профили конфигурации

| Профиль | RUNS | MAX_RUNS | OVERHEAD_RUNS | RAMP / STEADY / DOWN | COOLDOWN_MS | Назначение |
|---------|------|----------|---------------|----------------------|-------------|------------|
| **Final NIR** | 20 | 40 | 5–10 | 60 / 300 / 30 | 5000 | серия для отчёта |
| **Lab** | 10–20 | 20–40 | 3–5 | 60 / 300 / 30 | 2000 | проверка на Windows/Docker |
| **Debug** | 5 | 5–10 | 1 | 30 / 60 / 15 | 500 | быстрая проверка стенда |

Перед финальной серией: отключить pprof, не запускать параллельные Docker builds/IDE-heavy процессы, прогреть сам стенд dry-run запуском, зафиксировать commit, `.env.local` (или hash значимых `DWSS_*`), Docker image IDs, OS/CPU/Go version.

## Overhead — агрегация блоков

RTT агрегируются в **1-секундные блоки** (размер блока = `RPS`): медиана RTT в блоке. Блоки строятся **только из steady-фазы**.

Для финального overhead каждый повтор `overhead-mirror-off`/`overhead-mirror-on` даёт run-level P95 по block-medians. Итоговое сравнение: mirror on должен быть ≤ mirror off × 1.05. Timeseries block-medians — диагностический график стабильности.

## Изоляция и порядок

- Сценарии: `bench run --scenarios s0-control,s-ref,s-dw` или `all` (только три основных).
- Overhead — отдельная команда и каталог `results/overhead/`.
- `DWSS_BENCH_COOLDOWN_MS` — пауза между reset и probe / между runs.
- Не смешивать profiling (pprof) с замерами T_first.

## Manifest (`results.json`)

- `protocolVersion`, `gitCommit`, `GOMAXPROCS`, `startedAt`
- `gitDirty`, `goVersion`, `os`, `arch`, `numCPU`, `dockerVersion`, `dockerImages`
- URLs, Profile, runs, `runs[]` с per-run probe + warmkit state
- CV thresholds, `h4Runs`, `h4MaxRuns`, `envProfileName`, actual `readyAfter`
- `hypotheses`: H1, H2, H2_CI, H3 (для основных сценариев), `overhead` (для `bench run --scenarios overhead`)

## Критерии готовности к отчёту НИР

- Все графики сгенерированы из одного `results/full/results.json` и одного `results/overhead/results.json`; `figures/manifest.json` содержит sha256 этих файлов.
- `RUNS ≥ 20` для финальной серии.
- `nFiltered ≥ 0.8 × N` для каждого сценария; иначе серия считается слишком шумной или требует отдельного разбора.
- Для прогретых сценариев CV **≤ 20%**.
- H1/H2/H2_CI/H3 и overhead имеют verdict `pass`.
- Overhead выполнен повторяемой серией (`DWSS_BENCH_H4_RUNS > 1`) либо явно помечен как diagnostic-only.
- В отчёте указаны commit, профиль env, OS/CPU/Go/Docker, seed/samples/runs и правило фильтрации выбросов.

## Каталог графиков

- `fig07_load_profile` — методика нагрузки: ramp-up, steady, ramp-down.
- `fig01_tfirst_p50` — главный эффект по `T_first` (построение hashmap при первом запросе).
- `fig02_tfirst_runs` — разброс независимых run, filtered set и IQR-выбросы.
- `fig03_hypotheses_h1_h2` — сводная проверка H1/H2/H2_CI/H3.
- `fig04_cv_scenarios` — воспроизводимость и ограничения стенда.
- `fig05_h4_p95` — накладные расходы зеркалирования (p95 E2E).
- `fig06_h4_blocks_timeseries` — диагностический график стабильности steady-фазы overhead.

## Гипотезы

| ID | Формулировка (по median P50 T_first) |
|----|--------------------------------------|
| H1 | S_dw ≤ S0 / 2 |
| H2 | S_dw ≤ S_ref × 1.1 (P50) |
| H2_CI | upper bound P50 bootstrap CI для S_dw ≤ S_ref × 1.1 |
| H3 | `/readyz` = 200 после S_dw (логируется в `runs[].readyOk`) |
| overhead | p95 E2E proxy `/work`: mirror on ≤ off × 1.05 |

## CLI

```bash
make bench-s0
make bench-s-ref
make bench-s-dw
make bench-overhead
make bench-all    # S0 + S_ref + S_dw + hypotheses
```

```bash
cd benchmark-bench
go run ./cmd/bench run --scenarios s0-control --out ../results/s0
go run ./cmd/bench run --scenarios all --out ../results/full
go run ./cmd/bench run --scenarios overhead --out ../results/overhead
```

## Profiling

Отдельный прогон при CV > порога:

```bash
make profile-flame   # http://127.0.0.1:6060 — warmup pprof
```

См. [Flame Graphs for Go With pprof](https://www.benburwell.com/posts/flame-graphs-for-go-with-pprof/).

## Воспроизводимость

Перед серией прогонов: зафиксировать docker image ids, `.env.local`, commit. Результаты — в `Projects/results/` (gitignored).
