# LoadedService

HTTP-сервис, имитирующий нагруженный микросервис с cold start (mmap + lazy in-memory index).

Используется в СДПС (DWSS) в ролях `active` и `warmup`. На warmup встраивается библиотека **warmkit**.

## API

| Endpoint | Метод | Описание |
|----------|-------|----------|
| `/healthz` | GET | Liveness |
| `/readyz` | GET | Readiness (только `ROLE=warmup`, warmkit) |
| `/work` | GET | `?seed=&samples=` — боевой workload, `{duration_ns}` |
| `/warmup` | POST | JSON `WorkloadProfile` — эталонный прогрев (S_ref) |
| `/reset` | POST | Cold reset workload + warmkit (benchmark protocol v2) |
| `/state` | GET | Состояние workload + warmkit snapshot |

## Роли

| Env | Значение |
|-----|----------|
| `DWSS_ROLE` | `active` или `warmup` |
| `DWSS_INSTANCE_ID` | ID для ZK registry |
| `DWSS_HTTP_ADDR` | listen address (host:port) |

## Конфигурация

Все переменные обязательны — см. корневой `.env.example`. Значения только в `.env.local`.

## Запуск

```bash
# из LoadedService/ с загруженным .env.local
make run
make build
make test
```

## Docker

```bash
# из корня Projects/
docker compose -f deploy/docker-compose.yml --env-file .env.local up loaded-service-warmup
```

## Workload

- **OS cold start:** mmap файла (`DWSS_MMAP_FILE`, `DWSS_MMAP_SIZE_MB`)
- **App cold start:** lazy build индекса (`DWSS_INDEX_KEYS` ключей)

`/warmup` и `/work` с одним `WorkloadProfile` затрагивают **одинаковые** страницы и ключи (честный S_ref).

## pprof

При `DWSS_PPROF_ENABLED=true` — отдельный HTTP-сервер на `DWSS_PPROF_HTTP_ADDR`. См. корневой `Makefile` (`profile-cpu`, `profile-flame`).
