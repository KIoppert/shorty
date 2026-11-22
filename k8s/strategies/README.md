# Домашняя работа 1. Стратегии развёртывания Shorty

`shorty.localhost` принимает запросы через Envoy Gateway. HTTPRoute `shorty-web` направляет `/` и `/assets` в nginx. HTTPRoute `shorty-api` управляет остальным трафиком, включая REST API и короткие ссылки.

Два Deployment используют одну PostgreSQL с постоянным томом и один ключ подписи JWT. Каждый Deployment держит две реплики. `GET /healthz` проверяет связь с БД для readiness. `GET /api/livez` проверяет, что HTTP-сервер отвечает, для liveness. `GET /api/version` возвращает версию из сборки и имя пода. Все ответы API также содержат `X-Shorty-Version`.

| Deployment и Service | Образ перед демонстрацией | Назначение |
| --- | --- | --- |
| `api` | `shorty-api:v1` | Blue и цель RollingUpdate |
| `api-green` | `shorty-api:v2` | Green, канареечная версия, аудитория B и получатель зеркальной копии |

В приложенном семинарском демо используется та же схема GatewayClass → Gateway → HTTPRoute. В Shorty правила каждого этапа лежат в `routes/` и применяются к `shorty-api`. Синий пул здесь обозначает v1, зелёный — v2. Наблюдение за версиями идёт через HTTP-ответы и логи подов.

## 1. Подготовить кластер

Понадобятся Docker с BuildKit, Minikube с Docker-драйвером, kubectl, Bash, curl, jq и OpenSSL. Скрипт создаёт отдельный профиль `shorty-rollouts` с Kubernetes 1.34 и ставит Envoy Gateway 1.6.4. Для профиля выделяются 3 CPU и 4 ГБ памяти. Используются локальные настройки БД и JWT из базового Kubernetes-примера.

Из корня репозитория выполните:

```bash
git switch homework/deployment-strategies
set -o pipefail
export LOG_DIR="$HOME/shorty-evidence"
mkdir -p "$LOG_DIR"
./k8s/strategies/setup.sh 2>&1 | tee "$LOG_DIR/00-setup.log"
```

Скрипт собирает и загружает образы v1, v2 и nginx, применяет миграции и ждёт готовности Deployment и Gateway. Если Docker работает через TLS-прокси и сборка не видит его CA, перед запуском задайте `SHORTY_BUILD_CA` как путь к доверенному PEM-бандлу хоста. BuildKit передаст бандл на шаг загрузки Go-модулей.

## 2. Открыть HTTP-порт Envoy

В отдельном терминале выполните:

```bash
./k8s/strategies/traffic.sh
```

Скрипт находит Service Envoy, открывает порт 8080 и печатает версию и имя пода каждые полсекунды. Можно добавить `2>&1 | tee "$HOME/shorty-evidence/traffic.log"`, чтобы сохранить непрерывный поток. Для одного port-forward используйте `./k8s/strategies/traffic.sh --forward-only`. Оставьте выбранный процесс запущенным и откройте `http://shorty.localhost:8080` в браузере. Если порт занят локальным Compose, освободите его перед запуском.

## 3. Выполнить проверки и сохранить логи

В первом терминале выполните команды по очереди:

```bash
set -o pipefail
export LOG_DIR="$HOME/shorty-evidence"
./k8s/strategies/demo.sh baseline   2>&1 | tee "$LOG_DIR/01-baseline.log"
./k8s/strategies/demo.sh rolling    2>&1 | tee "$LOG_DIR/02-rolling.log"
./k8s/strategies/demo.sh blue-green 2>&1 | tee "$LOG_DIR/03-blue-green.log"
./k8s/strategies/demo.sh canary     2>&1 | tee "$LOG_DIR/04-canary.log"
./k8s/strategies/demo.sh ab         2>&1 | tee "$LOG_DIR/05-ab.log"
./k8s/strategies/demo.sh shadow     2>&1 | tee "$LOG_DIR/06-shadow.log"
kubectl --context shorty-rollouts -n shorty get gateway,httproute,deploy,rs,pods -o wide \
  | tee "$LOG_DIR/07-final-state.log"
```

Каждый сценарий начинает с blue v1, поэтому его можно повторить отдельно. Скрипт показывает правила HTTPRoute и проверяет HTTP-ответы. Если команда завершилась с ошибкой, устраните сбой и повторите её перед записью.

## 4. Записать скринкаст

До начала записи выполните шаги 1–2 и прогоните проверки. Для сдачи повторите шаг 3 при включённой записи, чтобы видео и логи относились к одному прогону.

В OBS Studio добавьте «Захват окна» терминала или «Захват экрана», выставьте 1920×1080 и 30 кадров/с. Увеличьте шрифт терминала так, чтобы были читаемы версии, имена подов и счётчики. Разместите рядом два терминала. В одном оставьте поток `traffic.sh`, в другом запускайте команды из шага 3. Включите запись и оставляйте каждый результат на экране на несколько секунд. Если OBS пишет MKV, после остановки используйте «Файл → Ремультиплексировать записи», чтобы получить MP4.

| Что записать | Что должно быть видно и что сказать |
| --- | --- |
| **Начало, baseline** | Gateway в состоянии Programmed, Deployment 2/2, пути обеих проб и два имени подов в ответах v1. Объясните, что данные хранятся в PostgreSQL. Можно коротко показать главную страницу в браузере. |
| **Rolling** | maxSurge 1 и maxUnavailable 0, смену чисел в строках старого и нового ReplicaSet, чередование v1/v2 в ответах во время обновления. В конце покажите 12 ответов v2. Маршрут всё это время указывает на Service api. |
| **Blue/Green** | Два готовых пула и ответы v1. Затем правило направляет трафик на api-green, все ответы идут от v2, а созданная на v1 ссылка доступна с той же сессией. После отката ответы снова v1 и оба пула по-прежнему 2/2. |
| **Canary** | Каждое правило и результат 120 запросов. Сначала около 90/10, затем около 50/50, в конце ровно 0 ответов v1 и 120 ответов v2. Первые два распределения вероятностные. |
| **A/B** | Три строки подряд. Без заголовка → v1, с x-user-group: beta → v2, с x-user-group: other → v1. |
| **Shadow** | Правило RequestMirror, клиентский ответ v1 и две JSON-строки из логов подов v1 и v2 с одинаковым demo_id. Зеркалируется только GET /api/version, который не записывает данные в БД. |

К сдаче приложите скринкаст MP4 и файлы `00-setup.log`–`07-final-state.log` из каталога `~/shorty-evidence`. В описании отметьте время начала каждой стратегии, если ролик записан одним файлом.
