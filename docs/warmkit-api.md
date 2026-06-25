# warmkit API

Библиотека `warmkit` управляет жизненным циклом **warmup-инстанса** (`ROLE=warmup`). На active-инстансе не используется.

## FSM

```
«Starting» → «Registered» → «Warming» → «Ready» → «Active»
                              ↘ «Failed»
```

| Состояние | Описание |
|-------|----------|
| «Starting» | Начальное состояние |
| «Registered» | Зарегистрирован в ZK |
| «Warming» | Получает теневой трафик (доля зеркалирования > 0) |
| «Ready» | `shadowCount` ≥ `ReadyAfter` |
| «Active» | Перевод координатором (`activeInstanceId`) |
| «Failed» | Таймаут или ошибка |

## Engine

```go
type Engine interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    State() State
    Snapshot() MetricsSnapshot
    ReadinessHandler() http.HandlerFunc
    ShadowMiddleware(next http.Handler) http.Handler
}
```

### Config

| Поле | Env (LoadedService) | Описание |
|------|---------------------|----------|
| ServiceName | DWSS_SERVICE_NAME | Имя приложения в ZK |
| InstanceID | DWSS_INSTANCE_ID | ID инстанса |
| ListenAddr | DWSS_HTTP_ADDR | Адрес для registry (host:port) |
| Role | DWSS_ROLE | `warmup` или `active` |
| ReadyAfter | DWSS_WARMUP_READY_AFTER | Порог теневых запросов для состояния «Ready» |
| WarmupTimeout | DWSS_WARMUP_TIMEOUT_SEC | Таймаут прогрева |
| ShadowMetricsKeep | DWSS_SHADOW_METRICS_KEEP | Размер окна latency |

## Shadow contract

- Заголовок: `X-Warmup-Shadow: 1`
- Только `GET` (мутации → 403)
- Middleware инкрементирует счётчик и записывает latency

## Registry

```go
type Registry interface {
    RegisterInstance(ctx context.Context, service, instanceID string, rec InstanceRecord) error
    UpdateInstanceState(ctx context.Context, service, instanceID string, state State) error
    WatchMirror(ctx context.Context, service string) (<-chan MirrorConfig, error)
    Close() error
}
```

- `ConnectZK(endpoints, timeout)` — production registry
- `NewMemoryRegistry()` — in-memory для тестов

## Mirror config (ZK)

Путь: `/config/{app}/mirror`

Writer: **только warmup-coordinator** (`WriteMirrorConfig`).

```go
type MirrorConfig struct {
    Enabled          bool
    Ratio            float64
    TargetInstanceID string
    ActiveInstanceID string
    SessionID        string
    UpdatedAtUnixMilli int64
}
```

## WorkloadProfile

```go
type WorkloadProfile struct {
    Seed    int64 `json:"seed"`
    Samples int   `json:"samples"`
}
```

Общий контракт с LoadedService и benchmark.

## MetricsSnapshot

```go
type MetricsSnapshot struct {
    State            State
    ShadowCount      int64
    ShadowLatencyP95 int64
    ReadyAfter       int
}
```

Экспортируется через `GET /state` на LoadedService.

## Интеграция в LoadedService

```go
if cfg.Role == "warmup" {
    reg, _ := warmkit.ConnectZK(cfg.ZKEndpoints, cfg.WarmupTimeout)
    wk = warmkit.NewEngine(cfg.WarmkitConfig(), reg, logger)
    wk.Start(ctx)
}
handler := wk.ShadowMiddleware(mux) // только warmup
mux.HandleFunc("GET /readyz", wk.ReadinessHandler())
```

## Тесты

```bash
cd warmkit && go test ./...
```

Покрытие: переходы FSM, shadow metrics P95, engine warmup→«Ready».
