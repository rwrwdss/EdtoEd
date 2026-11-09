# EdtoEd

EdtoEd — универсальный backend-модуль для LMS, который принимает события учебной активности, сохраняет их в PostgreSQL и возвращает риск академического отставания/оттока. Один и тот же сервис доступен через gRPC и REST; документация REST открывается в Swagger UI.

![CI](https://github.com/rwrwdss/EdtoEd/actions/workflows/ci.yml/badge.svg)

## Быстрый старт

```bash
docker compose up --build
```

После запуска:

- REST API: `http://localhost:8080`
- Swagger UI: `http://localhost:8080/docs`
- OpenAPI YAML: `http://localhost:8080/openapi.yaml`
- gRPC: `localhost:50051`
- liveness: `GET /healthz`
- readiness: `GET /readyz`

Пример REST-запроса:

```bash
curl -X POST http://localhost:8080/v1/analyze \
  -H 'Content-Type: application/json' \
  -d '{"student_id":"student_hash_123","topic_id":"algebra_01","clicks_count":24,"test_score":0.72,"deadline_delay":1.5}'
```

Ответ содержит `burnout_risk` от `0` до `1`, флаг `requires_action` и действие `KEEP_TRACK` или `OFFER_HINT`.

## Архитектура

```text
REST /v1/analyze ─┐
                  ├─ service.Analyzer ── features ── Predictor ── model
gRPC Analyze ─────┘          │
                             └────────── PostgreSQL event log
```

- `api/proto/` — gRPC-контракт и сгенерированные типы.
- `internal/httpapi/` — REST-маршруты, OpenAPI 3.0 и Swagger UI.
- `internal/service/` — общая бизнес-логика валидации, сохранения и scoring.
- `internal/features/` — единый порядок входных признаков.
- `internal/ml/` — интерфейс predictor и быстрый CGO-free predictor текущей ONNX-модели.
- `internal/storage/` — PostgreSQL и встроенная миграция.
- `models/` — ровно одна актуальная модель `edtoed_v1.onnx`.
- `ml_research/` — воспроизводимое обучение OULAD и отчёты.

## Модель

Текущая модель обучена на OULAD с окном первых 28 дней курса. Входной контракт: `[relative_clicks, test_score, deadline_delay]`. Целевая метка: `Fail`/`Withdrawn` против `Pass`/`Distinction`.

Последний тестовый результат:

| Метрика | Значение |
|---|---:|
| ROC-AUC | 0.783 |
| F1 | 0.720 |
| Accuracy | 0.708 |
| Recall | 0.710 |

Модель и preprocessing синхронизированы: клики и задержка проходят устойчивое `tanh`-преобразование и одинаковую нормализацию. Для переобучения:

```bash
cd ml_research
python3 train_oulad.py
cp artifacts/edtoed_oulad_v1.onnx ../models/edtoed_v1.onnx
```

Сырые данные и кэш Python не входят в Git. Production-папка `models/` не содержит старых версий.

## Конфигурация

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `DATABASE_URL` | обязательна | PostgreSQL DSN |
| `MODEL_PATH` | `models/edtoed_v1.onnx` | проверяемый путь модели |
| `GRPC_ADDR` | `:50051` | адрес gRPC |
| `HTTP_ADDR` | `:8080` | адрес REST/Swagger |

## Проверки разработчика

```bash
go test ./...
go vet ./...
gofmt -w $(find . -name '*.go' -not -path './api/proto/*')
```

GitHub Actions автоматически запускает форматирование, тесты, vet и сборку Docker при каждом push и pull request.

## Ограничения и следующий этап

Текущий predictor повторяет математику опубликованной лёгкой ONNX-модели нативно в Go, поэтому контейнер остаётся быстрым и не требует CGO. Для произвольных будущих ONNX-моделей понадобится отдельный runtime adapter с тем же интерфейсом `ml.Predictor`.

Ближайшие продуктовые шаги: реальные cohort-медианы кликов, подтверждённые production-метки, threshold tuning под стоимость ошибок, versioned model registry, OpenTelemetry и нагрузочные тесты.
