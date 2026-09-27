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
  migrations/    SQL-схема
web/
  public/        index.html и assets (css, js, шрифты, favicon)
  nginx.conf.template
docker-compose.yml
Отчёт.md         📝 отчёт по 12 факторам
```
