# НИР
Тема "Разработка системы динамического прогрева сервисов"

## Контекст и цель

Предметная область: при деплое нового инстанса микросервиса первые production-запросы попадают на «"холодный"» процесс (page cache, connection pools, JIT-кэши, lazy init). Это ухудшает latency и может вызвать каскадные таймауты.

Новый инстанс микросервиса прогревается **зеркалированием production-трафика** с автоматическим наращиванием доли mirror и переводом в active по критериям готовности warmkit.

Цель НИР: разработать систему, которая до включения в production прогревает новый 
инстанс зеркалированным трафиком (shadow traffic), координирует переход состояний 
через распределённое хранилище и даёт измеримый эффект на стенде.


## Архитектура

| Контур              | Назначение                                                          |
| ------------------- | ------------------------------------------------------------------- |
| **DWSS**            | warmkit, mirror-proxy, warmup-coordinator, LoadedService, ZooKeeper |
| **benchmark** | Эксперименты S0/S_ref/S_dw/H4, статистика, отчёты (не пишет в ZK)   

```plantuml
@startuml
left to right direction
skinparam shadowing false

title СДПС — C4 Container

actor "Client" as Client
rectangle "benchmark" as Bench #white;line:dashed
rectangle "СДПС" {
  rectangle "mirror-proxy" as Proxy
  rectangle "warmup-coordinator" as Coord
  rectangle "Active LoadedService" as Active
  rectangle "Warmup LoadedService\n+ warmkit" as Warmup
}
database "ZooKeeper" as ZK

Client --> Proxy
Bench --> Proxy
Bench ..> Coord : POST /v1/warmup/sessions
Proxy --> Active
Proxy ..> Warmup : mirror GET
Warmup --> ZK
Coord --> ZK : config/mirror
Proxy --> ZK
@enduml
```

## FSM warmkit

`Starting → Registered → Warming → Ready → Active | Failed`

## ZooKeeper

| ZNode | Writer | Readers |
|-------|--------|---------|
| `/services/{app}/instances/{id}` | warmkit | coordinator, proxy |
| `/config/{app}/mirror` | warmup-coordinator | mirror-proxy, warmkit |

## Быстрый старт

1. Скопировать `.env.example` → `.env.local` и заполнить **все** значения.
2. Поднять стенд:

```bash
make compose-up
```

3. Запустить эксперимент:

```bash
make bench-s0
make bench-s-ref
make bench-s-dw

make bench-all

make bench-overhead
```

## Сборка и тесты

```bash
make tidy
make test
make build
make lint
```

## Профилирование

```bash
make profile-cpu
make profile-flame
make profile-heap
```

## Структура репозитория

```
Projects/
├── warmkit/                  # библиотека прогрева
├── LoadedService/            # нагруженный микросервис
├── cmd/mirror-proxy/         # data plane
├── cmd/warmup-coordinator/   # control plane
├── benchmark/          # стенд бенчмаркинга
├── internal/envcfg/          # загрузка env без fallback
├── internal/pprofserver/     # опциональный pprof
├── deploy/                   # docker-compose
└── docs/                     # требования, архитектура, методика
```

## Документация

- [docs/requirements.md](requirements.md) — FR/NFR, гипотезы H1–H4
- [docs/architecture.md](architecture.md) — компоненты, ZK, ramp
- [docs/benchmark-methodology.md](benchmark-methodology.md) — S0/S_ref/S_dw, T_first
- [docs/warmkit-api.md](warmkit-api.md) — API библиотеки warmkit
- [docs/nir/](nir/) — тексты НИР 2 (исследование и отчёт)
- [LoadedService/README.md](../LoadedService/README.md) — HTTP API сервиса

## Конфигурация

В коде — **только имена** env-переменных. Числа, порты, seed — в `.env.local` (gitignored). Без numeric fallback.

## Метрика

**T_first** — latency первого боевого `GET /work` после прогрева. Сравнивается между S0 (контроль), S_ref (эталон прогрева), S_dw (dynamic warmup).
