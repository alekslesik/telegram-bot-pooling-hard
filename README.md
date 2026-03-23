# telegram-bot-simple

Демо-бот в Telegram: `https://t.me/TYygGIBZQUTFmqoA_bot`

Это демонстрационный Telegram‑бот, который можно показывать потенциальным заказчикам как «живой пример продукта».

## Что умеет бот (для заказчика)

- **Базовое общение**: бот принимает сообщения и отвечает (пример обработки обращений клиентов).
- **Информативные команды**:
  - **`/start`** — короткое приветствие и что делает бот
  - **`/help`** — список возможностей
  - **`/about`** — зачем такой бот бизнесу
  - **`/usecases`** — примеры задач (услуги, обучение, малый бизнес)
  - **`/features`** — какие функции можно добавить в такого бота (заявки, меню, запись, опросы и т.д.)
  - **`/ping`** — проверка, что бот онлайн
  - **`/echo <текст>`** — пример простой команды (повторяет текст)
  - **`/menu`** — текст «выберите пункт меню» и **inline-кнопки** под сообщением (те же разделы, что в основном меню)

## Три вида меню в боте (для демонстрации заказчику)

В боте одновременно показаны **три разных механизма** навигации в Telegram — они не заменяют друг друга, а дополняют:

| Вид | Где видно | Как настроено в коде |
|-----|-----------|----------------------|
| **1. Меню команд у поля ввода** | Кнопка/иконка меню рядом с полем ввода в клиенте Telegram — список slash-команд с описаниями | `setMyCommands` при старте в [`cmd/bot/main.go`](cmd/bot/main.go) |
| **2. Reply-клавиатура** | Постоянные кнопки **внизу экрана** над полем ввода (как «быстрый доступ») | `ReplyKeyboardMarkup` в [`internal/bot/handlers.go`](internal/bot/handlers.go) (`commandKeyboard()`), прикрепляется к ответам бота |
| **3. Inline-клавиатура** | Кнопки **под конкретным сообщением** бота (не двигают нижнюю панель) | `InlineKeyboardMarkup` в том же файле (`demoInlineMenuKeyboard()`), показывается в ответ на **`/menu`** (и при нажатии inline вызываются те же действия, что и у команд) |

**Важно:** в одном сообщении Telegram позволяет только **один** тип `reply_markup`. Поэтому сообщение с `/menu` несёт **inline-меню**; нижняя reply-клавиатура остаётся от предыдущих ответов бота, пока её не скрыть.

## Для каких задач лучше выбрать Telegram-бота

- **Поддержка клиентов 24/7**: ответы на частые вопросы, выдача инструкций.
- **Сбор заявок и контактов**: «оставьте телефон / комментарий» прямо в чате.
- **Автоматизация типовых сценариев**: прайсы, расписание, анкеты, опросы, напоминания.
- **Лёгкий вход в продукт**: Telegram не требует установки приложения — клиент уже там.

## Быстрый старт (для разработки)

### Требования

- Go 1.26+
- Docker + Docker Compose (опционально)
- Аккаунт Telegram и созданный бот через `@BotFather`

### Настройка `.env`

1. Скопируй пример:

```bash
cp .env.example .env
```

2. Заполни `TOKEN` и `USERNAME`.

Переменные:

- **`TOKEN`**: токен бота от `@BotFather`
- **`USERNAME`**: username бота (без `@`)
- **`LOG_LEVEL`**: `debug` или `info` (по умолчанию `info`)
- **`LOG_FORMAT`**: `json` или `text` (по умолчанию `text`)

### Запуск локально

```bash
make run
```

### Тесты

```bash
make test
```

## Запуск в Docker

```bash
make docker-run
```

## Запуск через Docker Compose

```bash
make docker-compose-up
```

Остановить:

```bash
make docker-compose-down
```

## CI/CD: GHCR + автодеплой на VPS по SSH

В репозитории настроены workflow’ы GitHub Actions:

- **CI**: запускает проверки как в `Makefile` (форматирование, линтеры, тесты, vuln, docker build).
- **Release**: собирает Docker-образ и пушит в GitHub Container Registry (GHCR) `ghcr.io/<owner>/<repo>`.
- **Deploy**: после успешного релиза автоматически деплоит на VPS по SSH.

### Подготовка VPS (один раз)

1. Установи Docker и Docker Compose на сервер.
2. Создай директорию для бота, например:

```bash
sudo mkdir -p /opt/bots/telegram-bot-simple
sudo chown -R $USER:$USER /opt/bots/telegram-bot-simple
cd /opt/bots/telegram-bot-simple
```

3. Скопируй на сервер файл `.env` (секреты храним на сервере, не в git). `docker-compose.prod.yaml` будет доставляться автодеплоем.

```bash
scp .env user@server:/opt/bots/telegram-bot-simple/
```

### Что нужно сделать на VPS (кратко)

- **Установить Docker + Compose (Ubuntu/Debian)**:

```bash
sudo apt update
sudo apt install -y ca-certificates curl
curl -fsSL https://get.docker.com | sudo sh
sudo apt install -y docker-compose-plugin
sudo systemctl enable --now docker
```

- **Подготовить папку приложения** (должны лежать `docker-compose.prod.yaml` и `.env`):

```bash
sudo mkdir -p /opt/bots/telegram-bot-simple
sudo chown -R $USER:$USER /opt/bots/telegram-bot-simple
```

- **Дать пользователю доступ к Docker** (чтобы деплой по SSH мог запускать `docker compose`):

```bash
sudo usermod -aG docker $USER
newgrp docker
```

- **Быстрая проверка вручную на VPS**:

```bash
cd /opt/bots/telegram-bot-simple
docker login ghcr.io -u "<GHCR_READ_USER>" -p "<GHCR_READ_TOKEN>"
IMAGE_TAG=v1.2.3 docker compose -f docker-compose.prod.yaml pull
IMAGE_TAG=v1.2.3 docker compose -f docker-compose.prod.yaml up -d
docker ps
```

### Секреты GitHub для деплоя

В Settings → Secrets and variables → Actions добавь:

- **`VPS_HOST`**: IP/домен сервера
- **`VPS_USER`**: пользователь (например `root` или `deploy`)
- **`VPS_SSH_KEY`**: приватный SSH ключ (PEM), которым GitHub будет подключаться
- **`VPS_APP_PATH`**: путь на сервере, например `/opt/bots/telegram-bot-simple`
- **`GHCR_READ_USER`**: логин GitHub (владелец пакета)
- **`GHCR_READ_TOKEN`**: токен с правами `read:packages` (для `docker login ghcr.io` на сервере)

#### Где взять `VPS_SSH_KEY`

Сгенерируй отдельную пару ключей для деплоя на локальной машине:

```bash
ssh-keygen -t ed25519 -C "gh-actions-deploy" -f ~/.ssh/gh_actions_vps
```

Публичный ключ добавь на VPS в `~/.ssh/authorized_keys` пользователя, под которым будет деплой:

```bash
ssh-copy-id -i ~/.ssh/gh_actions_vps.pub deploy@<VPS_HOST>
```

В GitHub Secret **`VPS_SSH_KEY`** вставь **содержимое приватного** `~/.ssh/gh_actions_vps` (целиком).

#### Где взять `GHCR_READ_TOKEN`

Создай GitHub Personal Access Token (classic) и включи scope **`read:packages`**.
Если GHCR-пакет приватный и `docker pull` не проходит — добавь также scope **`repo`**.

### Версии и теги Docker-образа

- **`vX.Y.Z`**: релизный тег (SemVer). Образ публикуется как `ghcr.io/<owner>/<repo>:vX.Y.Z`.

Схема релиза:

1) Создай git-тег:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

2) В GitHub открой **Releases** → **Draft a new release** → выбери тег `v1.2.3` → нажми **Publish release**.

Внутри контейнера версия прошивается в бинарь (через `-ldflags`) и печатается при старте как `version/commit/build_date`.

### Как работает деплой

- Деплой запускается **только после публикации GitHub Release** (кнопка **Publish release**) и деплоит образ `:vX.Y.Z` на VPS.

На сервере используется `docker-compose.prod.yaml`, который запускает образ из GHCR:

```yaml
services:
  bot:
    image: ghcr.io/alekslesik/telegram-bot-simple:${IMAGE_TAG:-latest}
```

## Деплой на VPS (общая схема)

1. Создать VPS у любого провайдера с установленным Docker.
2. Положить на сервер `.env` (секреты) и `docker-compose.prod.yaml` (compose может обновляться автодеплоем).
3. На сервере:

```bash
cd /opt/bots/telegram-bot-simple
IMAGE_TAG=v1.2.3 docker compose -f docker-compose.prod.yaml pull
IMAGE_TAG=v1.2.3 docker compose -f docker-compose.prod.yaml up -d
```

Бот использует long polling, поэтому обычно не требуется публичный HTTPS и настройка webhook — достаточно, чтобы сервер имел доступ в интернет.
