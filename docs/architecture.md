# Архитектура СДПС (DWSS)

## Два контура

| Контур | Компоненты | Ответственность |
|--------|------------|-----------------|
| **DWSS** | warmkit, mirror-proxy, warmup-coordinator, LoadedService, ZooKeeper | Динамический прогрева нового инстанса production-трафиком |
| **benchmark** | experiment, loadgen, probe, stats, report | Протоколы S0/S_ref/S_dw/H4, статистика, отчёты |

benchmark **не пишет** в ZooKeeper. Для S_dw вызывает HTTP API coordinator.

## C4 Container Diagram

```plantuml
@startuml
left to right direction
skinparam shadowing false

title Система динамического прогрева сервисов (СДПС)

actor "Production\nClient" as Client
rectangle "Benchmark Bench" as BenchCont #white;line:dashed {
  rectangle "benchmark" as Bench
}

rectangle "СДПС" {
  rectangle "mirror-proxy" as Proxy
  rectangle "warmup-coordinator" as Coord
  rectangle "Active\nLoadedService" as Active
  rectangle "Warmup\nLoadedService + warmkit" as Warmup
}

database "ZooKeeper" as ZK

Client --> Proxy : production HTTP
Bench --> Proxy : load (эксперимент)
Bench ..> Coord : POST /v1/warmup/sessions
Proxy --> Active : forward
Proxy ..> Warmup : async GET mirror\nX-Warmup-Shadow: 1
Warmup --> ZK : instances/{id}
Coord --> ZK : config/mirror (writer)
Proxy --> ZK : read mirror config
@enduml
```

## Компоненты

### warmkit (`warmkit/`)

Библиотека для инстанса с `ROLE=warmup`:

- FSM: `Starting → Registered → Warming → Ready → Active | Failed`
- Ephemeral-регистрация в `/services/{app}/instances/{id}`
- Shadow middleware: счётчик GET, запрет мутаций при `X-Warmup-Shadow: 1`
- Readiness handler для `/readyz`

### mirror-proxy (`cmd/mirror-proxy/`)

Data plane: forward на active, async mirror GET на warmup по конфигу ZK.

### warmup-coordinator (`cmd/warmup-coordinator/`)

Control plane: единственный writer `/config/{app}/mirror`, ramp ratio, promote active.

### LoadedService (`LoadedService/`)

Имитация нагруженного микросервиса (mmap + lazy index). Роли `active` и `warmup` через env.

### benchmark (`benchmark/`)

Стенд бенчмаркинга: T_first, median, p95, bootstrap CI, manifest.json.

## ZooKeeper

```
/services/{app}/instances/{instanceId}   # ephemeral, writer: warmkit
/config/{app}/mirror                     # persistent, writer: warmup-coordinator
```

## Динамический прогрев

1. Coordinator создаёт сессию (`POST /v1/warmup/sessions`).
2. Записывает mirror config: `enabled=true`, `ratio=0`, `targetInstanceId`.
3. Каждые `DWSS_COORD_RAMP_INTERVAL_SEC` увеличивает `ratio` по `DWSS_COORD_RAMP_STEPS`.
4. warmkit на warmup при `shadowCount ≥ ReadyAfter` → `Ready`, `/readyz=200`.
5. Coordinator переводит `activeInstanceId` на warmup, `ratio=0`.
6. Сессия `completed`.

## Конфигурация

Все числовые значения — в `.env.local` (gitignored). В коде только имена переменных, без fallback.

См. также: [requirements.md](requirements.md), [warmkit-api.md](warmkit-api.md), [benchmark-methodology.md](benchmark-methodology.md).
