# ✂️ shorty

Сокращатель ссылок: регистрируетесь, вставляете длинную ссылку и получаете короткую с QR-кодом и статистикой переходов 🎟️

## ✨ Что умеет

- 🔐 Регистрация и вход (JWT в HttpOnly-cookie)
- 🔗 Короткие ссылки со случайным кодом или своим алиасом (`/my-cv`)
- 🏷️ Название для ссылки, чтобы не путаться
- ⏳ Срок действия: после него ссылка отдаёт `410 Gone`
- 🔌 Включение и выключение без удаления
- 📊 Статистика: всего переходов, график за 14 дней, последние переходы (источник и устройство)
- 📱 QR-код для каждой ссылки со скачиванием в PNG
- 🌗 Светлая и тёмная тема по системной настройке
- 🗑️ Удаление аккаунта вместе со всеми данными
- 🛡️ Админка: сводка по сервису, все пользователи и все ссылки, поиск, бан и разбан, выдача прав админа, отключение и удаление чужих ссылок

## 🧱 Стек

| | |
|---|---|
| 🐹 Бэкенд | Go 1.27, `net/http`, `pgx`, `golang-jwt`, `bcrypt`, `go-qrcode` |
| 🐘 База | PostgreSQL 17 |
| 🎨 Фронтенд | HTML + CSS + vanilla JS без сборки, шрифты лежат локально, раздаёт nginx |
| 🐳 Запуск | Docker Compose |

## 🚀 Запуск

```bash
cp .env.example .env
# поменяйте JWT_SECRET на случайную строку: openssl rand -hex 32
docker compose up --build
```

Откройте 👉 http://localhost:8080

## 🛡️ Как стать админом

Зарегистрируйтесь в интерфейсе, затем выдайте себе права разовой командой:

```bash
docker compose run --rm api promote you@example.com
```

Обновите страницу, и в шапке появится вкладка «Админка» 👑 Дальше права можно выдавать другим пользователям прямо из админки.

Что происходит при бане 🚫:

- все сессии пользователя сразу перестают работать, войти заново нельзя;
- все короткие ссылки пользователя начинают отвечать `404`;
- данные остаются на месте, так что разбан возвращает всё как было.

## 🗺️ Как устроено

```
браузер ──► web (nginx :8080) ──┬── /            → index.html
                                ├── /assets/*    → статика
                                └── всё остальное → api (Go :8080) ──► PostgreSQL
```

| Сервис | Что делает |
|---|---|
| `db` | PostgreSQL, данные в volume `pgdata` |
| `migrate` | Разовый процесс: применяет миграции и завершается |
| `api` | REST API и редиректы `/{code}` |
| `web` | Раздаёт фронт и проксирует запросы в `api` |

## ⚙️ Переменные окружения

| Переменная | По умолчанию | Зачем |
|---|---|---|
| `RELEASE` | `dev` | Тег образов, например git-хеш коммита |
| `DATABASE_URL` | собирается из `POSTGRES_*` | Строка подключения к Postgres, можно указать внешнюю базу |
| `JWT_SECRET` | — | Секрет подписи токенов, минимум 32 символа |
| `TOKEN_TTL` | `168h` | Сколько живёт сессия |
| `COOKIE_SECURE` | `false` | `true`, если сайт работает по HTTPS |
| `BASE_URL` | `http://localhost:8080` | Публичный адрес для коротких ссылок и QR |
| `PORT` | `8080` | Порт, который слушает `api` |
| `FAULT_ERROR_RATE` | `0` | Доля запросов, на которые `api` нарочно отвечает 500 (для демо SLO) |
| `FAULT_DELAY` | `0s` | Искусственная задержка каждого запроса (для демо SLO) |
| `HTTP_PORT` | `8080` | Порт `web` на хосте |

## 📡 API

| Метод | Путь | Что делает |
|---|---|---|
| `POST` | `/api/auth/register` | Регистрация |
| `POST` | `/api/auth/login` | Вход |
| `POST` | `/api/auth/logout` | Выход |
| `GET` | `/api/me` | Текущий пользователь |
| `DELETE` | `/api/me` | Удалить аккаунт |
| `GET` | `/api/links` | Мои ссылки |
| `POST` | `/api/links` | Создать ссылку |
| `GET` | `/api/links/{id}` | Одна ссылка |
| `PUT` | `/api/links/{id}` | Изменить ссылку |
| `DELETE` | `/api/links/{id}` | Удалить ссылку |
| `GET` | `/api/links/{id}/stats` | Статистика переходов |
| `GET` | `/api/links/{id}/qr` | QR-код в PNG |
| `GET` | `/api/admin/overview` | 🛡️ Сводка по сервису |
| `GET` | `/api/admin/users?q=` | 🛡️ Все пользователи с поиском |
| `PATCH` | `/api/admin/users/{id}` | 🛡️ Бан, разбан, права админа: `{"banned": true}`, `{"is_admin": true}` |
| `DELETE` | `/api/admin/users/{id}` | 🛡️ Удалить пользователя |
| `GET` | `/api/admin/links?q=` | 🛡️ Все ссылки с поиском |
| `PATCH` | `/api/admin/links/{id}` | 🛡️ Включить или выключить ссылку |
| `DELETE` | `/api/admin/links/{id}` | 🛡️ Удалить ссылку |
| `GET` | `/{code}` | Редирект на оригинал |
| `GET` | `/healthz` | Проверка живости (пингует базу) |

Пример:

```bash
curl -c jar -X POST localhost:8080/api/auth/register -d '{"email":"me@example.com","password":"password123"}'
curl -b jar -X POST localhost:8080/api/links -d '{"url":"https://go.dev","alias":"go"}'
curl -i localhost:8080/go
```

## ☸️ Kubernetes (minikube)

```bash
./run.sh   # 🚀 minikube + ingress + сборка образов + деплой + ожидание готовности
kubectl -n ingress-nginx port-forward svc/ingress-nginx-controller 8080:80   # 🚇 в отдельном терминале
```

Откройте 👉 http://shorty.localhost:8080

| Команда | Что делает |
|---|---|
| `kubectl -n shorty get pods,svc,ingress` | 👀 что запущено |
| `kubectl -n shorty exec deploy/api -- shorty promote you@example.com` | 👑 выдать права админа |
| `kubectl -n shorty scale deployment/api --replicas=4` | 📈 больше реплик api |
| `kubectl -n shorty logs -l app.kubernetes.io/name=api --prefix --tail=5` | 📜 какая реплика обработала запросы |
| `kubectl delete -f k8s/` | 🧹 удалить всё вместе с данными |

Пароли для кластера лежат в `k8s/11-secrets.yaml` 🔑 Подробности — в [k8s/Отчёт.md](k8s/Отчёт.md) 📝

## 📜 Логи (VictoriaLogs)

```bash
kubectl apply -f k8s/logging/
kubectl -n logging port-forward svc/victorialogs 9428:9428   # 👉 http://localhost:9428/select/vmui/
```

Приложение пишет JSON в stdout, Vector (DaemonSet) собирает логи всех подов из namespace `shorty` и отправляет их в VictoriaLogs 🚚 Нагенерировать трафик для демо: `./k8s/logging/traffic.sh` 🔎 Подробности — в [k8s/logging/Отчёт.md](k8s/logging/Отчёт.md)

## 📊 Метрики (VictoriaMetrics)

```bash
helm repo add vm https://victoriametrics.github.io/helm-charts/ && helm repo update
helm install vm-single vm/victoria-metrics-single -n monitoring --create-namespace -f k8s/monitoring/vm-values.yaml
kubectl port-forward svc/vm-single-victoria-metrics-single-server -n monitoring 8428:8428   # 👉 http://localhost:8428/vmui
```

`api` отдаёт метрики рантайма Go, HTTP и бизнес-метрики на `:9091/metrics`, PostgreSQL через `postgres-exporter` на `:9187` 📈 VictoriaMetrics находит их сама по аннотациям `prometheus.io/*`. Подробности — в [k8s/monitoring/Отчёт.md](k8s/monitoring/Отчёт.md)

## 🎯 SLO

```bash
./k8s/slo/load.sh                                                      # 🚦 постоянная нагрузка
kubectl -n shorty set env deployment/api FAULT_ERROR_RATE=0.2          # 💥 20% ответов 500
kubectl -n shorty set env deployment/api FAULT_ERROR_RATE- FAULT_DELAY=400ms   # 🐢 медленные ответы
kubectl -n shorty set env deployment/api FAULT_DELAY-                  # ✅ вернуть как было

kubectl apply -f k8s/slo/dashboard.yaml                                # 📊 дашборд для vmui
helm upgrade vm-single vm/victoria-metrics-single -n monitoring -f k8s/monitoring/vm-values.yaml -f k8s/slo/vm-dashboard-values.yaml
```

Дашборд: 👉 http://localhost:8428/vmui/#/dashboards

Пробер (`shorty probe`, Deployment `prober`) каждые 10 секунд открывает фронтенд, `/healthz` и короткую ссылку и отдаёт результаты в VictoriaMetrics 🤖 Формулы SLI, SLO и SLA и графики — в [k8s/slo/Отчёт.md](k8s/slo/Отчёт.md)

## 🛠️ Полезные команды

```bash
docker compose logs -f api            # 📜 логи в JSON
docker compose run --rm migrate       # 🧬 применить миграции вручную
docker compose run --rm api promote me@example.com   # 👑 выдать права админа
RELEASE=$(git rev-parse --short HEAD) docker compose build   # 🏷️ собрать релиз с тегом коммита
docker compose up -d --scale api=3    # 📈 запустить три инстанса api
docker compose restart web            # 🔁 чтобы nginx увидел новые инстансы
```

## 📂 Структура

```
api/
  main.go        запуск, команды serve, migrate и promote, graceful shutdown
  config.go      конфиг из переменных окружения
  migrate.go     встроенные SQL-миграции
  store.go       запросы к PostgreSQL
  auth.go        пароли, JWT, регистрация и вход, проверка бана и прав
  links.go       CRUD ссылок, статистика, QR, редирект
  admin.go       эндпоинты админки
  http.go        роутинг, JSON-хелперы, логирование запросов
  metrics.go     метрики Prometheus: рантайм, HTTP, бизнес
  probe.go       пробер: синтетические проверки для SLI
  faults.go      управляемые сбои для демонстрации SLO
  migrations/    SQL-схема
k8s/             манифесты Kubernetes (по порядку применения) и отчёт по ним
  logging/       VictoriaLogs, Vector, скрипт трафика и отчёт по логам
  monitoring/    values для VictoriaMetrics и отчёт по метрикам
  slo/           нагрузка и отчёт по SLI, SLO и SLA
web/
  public/        index.html и assets (css, js, шрифты, favicon)
  nginx.conf.template
docker-compose.yml
run.sh           🚀 полный сценарий деплоя в minikube
Отчёт.md         📝 отчёт по 12 факторам
```
