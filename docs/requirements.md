# Требования — система динамического прогрева сервисов (СДПС)

## Цель

Разработать и экспериментально оценить систему динамического прогрева микросервисов: новый инстанс прогревается зеркалированием production-трафика с автоматическим наращиванием доли mirror и переводом в active по критериям готовности.

## Задачи исследования

1. Спроектировать архитектуру СДПС (warmkit, mirror-proxy, warmup-coordinator, benchmark-bench).
2. Реализировать прототип библиотеки warmkit и интеграцию в LoadedService.
3. Построить стенд бенчмаркинга с протоколами S0, S_ref, S_dw.
4. Экспериментально проверить гипотезы H1–H4.

## Функциональные требования

| ID | Требование |
|----|------------|
| FR-1 | mirror-proxy маршрутизирует production HTTP на active и асинхронно зеркалирует GET на warmup |
| FR-2 | warmup-coordinator управляет ramp mirror ratio и promote ACTIVE; единственный writer конфига mirror в ZK |
| FR-3 | warmkit ведёт FSM warmup-инстанса, регистрацию в ZK, readiness (`/readyz`), shadow middleware |
| FR-4 | LoadedService имитирует cold start (mmap + lazy cache) через `/work`, `/warmup` и `POST /reset` для независимых cold-run |
| FR-5 | benchmark-bench выполняет протокол по benchmark-methodology.md (S0/S_ref/S_dw/H4): reset → warmup → один probe; без записи в ZK |

## Нефункциональные требования

| ID | Требование |
|----|------------|
| NFR-1 | Конфигурация только через `.env.local`; в коде — имена переменных, без numeric fallback |
| NFR-2 | Async mirror не блокирует ответ клиенту на active (H4: overhead ≤ 5%) |
| NFR-3 | Воспроизводимость: фиксированный WorkloadProfile, manifest.json, CV ≤ 5% для ключевых прогонов |

## WorkloadProfile

```json
{ "seed": "<int64>", "samples": "<int>" }
```

Единый профиль для S0, S_ref, S_dw и probe T_first.

## Первичная метрика

**T_first** — `duration_ns` первого и единственного боевого `GET /work` (без shadow) после фазы прогрева в **независимом run** (`POST /reset` перед каждым run).

## Гипотезы

| ID | Формулировка |
|----|--------------|
| H1 | S_dw.T_first ≤ S0.T_first / 2 |
| H2 | S_dw.T_first ≤ S_ref.T_first × 1.1 |
| H3 | Coordinator фиксирует Registered→Ready; `/readyz` совпадает с warmkit |
| H4 | p95 active при mirror on ≤ p95 без mirror × 1.05 |

## Ограничения

- Shadow mirror только для идемпотентных GET.
- Profiling (pprof) на отдельном прогоне, не смешивать с T_first.
- ZooKeeper обязателен для полного стенда (docker-compose).
