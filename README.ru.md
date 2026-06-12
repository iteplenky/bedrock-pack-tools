# bedrock-pack-tools

[English](README.md) | Русский

[![Release](https://img.shields.io/github/v/release/iteplenky/bedrock-pack-tools?logo=github&sort=semver)](https://github.com/iteplenky/bedrock-pack-tools/releases/latest)
[![Test](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml/badge.svg)](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iteplenky/bedrock-pack-tools/v3.svg)](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3)
[![License: MIT](https://img.shields.io/github/license/iteplenky/bedrock-pack-tools)](LICENSE)

**Извлекайте AES-ключи контента, скачивайте и расшифровывайте зашифрованные
ресурс-паки Minecraft Bedrock из командной строки.** Кроссплатформенная CLI на
Go для исследователей безопасности, операторов серверов, проверяющих
собственные развёртывания, и авторов паков, восстанавливающих свои ключи -
снимайте ключи прямо с живого сервера Bedrock через Xbox Live, не имея ключа
заранее, а затем превращайте зашифрованные файлы `.mcpack` в обычные
редактируемые папки или зашифровывайте свои собственные.

- **`keys`** - вход через Xbox Live и извлечение AES-ключей контента сервера в `keys.json`, ключ заранее не нужен
- **`download`** - скачивание всех ресурс-паков, которые отдаёт сервер Bedrock, вместе с ключами, одной командой (добавьте `--decrypt` для готовых к использованию папок)
- **`decrypt`** - превращение зашифрованного `.mcpack` в обычный редактируемый каталог
- **`encrypt`** - упаковка обычного ресурс-пака в готовый к развёртыванию `.mcpack` + `.mcpack.key`
- **`featured`** - просмотр и скачивание из каталога Featured Servers / Live Events в Minecraft
- **`login`** / **`logout`** - вход через Xbox Live (поток device code) или очистка кэшированных токенов
- **`version`** - вывод версии сборки
- **интерактивное меню** - запуск без команды открывает меню с разделами: просмотр Featured Servers, ввод любого `host:port` с выбором действия, расшифровка прошлых загрузок, шифрование локального пака или переход в Settings - с наблюдением за прогрессом в реальном времени (`p` ставит на паузу, `esc` отменяет)

**Назначение.** Создано для исследователей, операторов серверов, проверяющих
собственные развёртывания, и авторов паков, восстанавливающих свои ключи. Не для
распространения чужого платного контента.

## Что это делает одной командой

```bash
# Скачать все зашифрованные паки, которые отдаёт сервер, затем расшифровать их за один шаг:
bedrock-pack-tools download --decrypt play.example.net:19132
```

Вы получаете обычные редактируемые папки паков плюс `keys.json` (UUID к AES-ключу) -
готовые к просмотру, сравнению или переупаковке. Запустите без аргументов, чтобы
вместо этого открыть интерактивное меню.

## Содержание

- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Команды](#команды)
- [FAQ](#faq)
- [Устранение неполадок](#устранение-неполадок)
- [Переменные окружения](#переменные-окружения)
- [Безопасность](#безопасность)
- [Участие в разработке](#участие-в-разработке)
- [Лицензия](#лицензия)

## Установка

### Готовый бинарник (быстрее всего, без инструментария Go)

Возьмите архив для вашей машины со
[страницы Releases](https://github.com/iteplenky/bedrock-pack-tools/releases/latest):

| Ваша машина | Архив |
|---|---|
| macOS, Apple Silicon (M1/M2/M3) | `bedrock-pack-tools_darwin_arm64.tar.gz` |
| macOS, Intel | `bedrock-pack-tools_darwin_amd64.tar.gz` |
| Linux, большинство ПК и серверов | `bedrock-pack-tools_linux_amd64.tar.gz` |
| Linux, ARM (например, Raspberry Pi 64-бит) | `bedrock-pack-tools_linux_arm64.tar.gz` |
| Windows, большинство ПК | `bedrock-pack-tools_windows_amd64.zip` |
| Windows, ARM | `bedrock-pack-tools_windows_arm64.zip` |

Скачайте и распакуйте (подставьте имя своего архива):

```bash
curl -L https://github.com/iteplenky/bedrock-pack-tools/releases/latest/download/bedrock-pack-tools_darwin_arm64.tar.gz | tar xz
./bedrock-pack-tools --help
```

Windows: распакуйте `.zip` и выполните `bedrock-pack-tools.exe --help`. Сверьте
любую загрузку с `checksums.txt` со страницы Releases (`sha256sum -c checksums.txt`).

### Из исходников (Go 1.25+)

```bash
go install github.com/iteplenky/bedrock-pack-tools/v3@latest
```

Суффикс `/v3` обязателен. Бинарник попадёт в `$(go env GOPATH)/bin` - при необходимости добавьте этот путь в `PATH`.

### Вход через Xbox Live

Командам `keys`, `download` и `featured` нужен аккаунт Microsoft / Xbox Live.
При первом запуске вы увидите запрос device code:

```
Auth: no cached token - starting Xbox Live device auth
A URL and code will appear - enter it in your browser.
```

Токен кэшируется локально и переиспользуется при последующих запусках. Команда
`featured` также получает MCToken через PlayFab, который кэшируется отдельно
примерно на 4 часа. Файлы кэша (`.xbox_token.json`, `.mctoken.json`,
`.device_id`) находятся в каталоге пользовательских настроек ОС:

- macOS: `~/Library/Application Support/bedrock-pack-tools/`
- Linux: `~/.config/bedrock-pack-tools/`
- Windows: `%AppData%\bedrock-pack-tools\`

## Быстрый старт

Запустите без аргументов, чтобы открыть интерактивное меню - выберите сервер из
Featured или введите любой `host:port`, выберите действие (скачать, скачать +
расшифровать или только ключи) и следите за прогрессом на месте (`p` ставит на
паузу, `esc` отменяет). В меню также есть **Decrypt packs**, **Encrypt a pack** и
**Settings** (вход/выход, очистка сохранённых адресов или истории загрузок, сброс
когорты featured):

```bash
./bedrock-pack-tools
```

Из командной строки:

```bash
./bedrock-pack-tools featured                                   # посмотреть, что доступно сейчас
./bedrock-pack-tools download --decrypt play.example.net:19132  # скачать + расшифровать за один шаг
```

## Команды

| Команда | Назначение | Пример |
| --- | --- | --- |
| `keys` | Снять AES-ключи контента сервера в `keys.json` | `bedrock-pack-tools keys play.example.net:19132` |
| `download` | Скачать все паки, которые отдаёт сервер, плюс ключи | `bedrock-pack-tools download --decrypt play.example.net:19132` |
| `decrypt` | Превратить зашифрованный пак в обычную редактируемую папку | `bedrock-pack-tools decrypt --all keys.json packs/` |
| `encrypt` | Упаковать обычный пак в `.mcpack` + `.mcpack.key` | `bedrock-pack-tools encrypt my-pack/` |
| `featured` | Просмотр и скачивание из каталога Featured Servers / Live Events | `bedrock-pack-tools featured` |
| `login` | Войти через Xbox Live и кэшировать токен | `bedrock-pack-tools login` |
| `logout` | Удалить кэшированные файлы авторизации Xbox + MCToken | `bedrock-pack-tools logout` |
| `version` | Вывести версию сборки | `bedrock-pack-tools version` |

Несколько флагов, которые не помещаются в таблицу:

- `download` складывает вывод каждого сервера в отдельную папку `<server>/` (паки,
  `keys.json` и, с `--decrypt`, подпапку `decrypted/`), чтобы загрузки с разных
  серверов не сваливались в кучу и не конфликтовали по имени пака. `--decrypt`
  расшифровывает каждый пак сразу; без флага инструмент выводит команду `decrypt --all`,
  которую нужно запустить дальше.
- `decrypt --all <keys.json> <packs-dir>` расшифровывает пакетно, сопоставляя паки
  с ключами по UUID в `manifest.json` каждого пака, поэтому структура каталогов не важна.
- `encrypt` генерирует свежий 32-символьный ключ AES-256-CFB8 на каждый файл;
  передайте свой ключ или используйте `--key-out PATH`, чтобы записать мастер-ключ
  вне каталога пака.
- Строки `featured` помечаются `[ON]`/`[OFF]` (публичный `host:port`, результат
  RakNet-пинга), `[EXP]` (разрешается по `experienceId` при скачивании) или `[EVT]`
  (live event, разрешается при скачивании).

Выполните `bedrock-pack-tools <command> --help` для полного синтаксиса и опций или
загляните в [godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3)
за внутренним устройством.

### `keys.json`

Карта UUID пака -> информация о шифровании, записывается командами `keys` /
`download` и используется `decrypt --all`:

```json
{
  "12345678-1234-1234-1234-123456789abc": {
    "key": "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
    "version": "1.0.0",
    "name": "Example Pack"
  }
}
```

Пофайловые ключи находятся внутри `contents.json` каждого зашифрованного пака;
инструмент читает их автоматически, как только получит мастер-ключ. Бинарный
формат зашифрованного пака описан в
[godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3).

## FAQ

**Как преобразовать `.mcpack` в `.zip`?**
`.mcpack` - это zip-архив, переименуйте расширение в `.zip` и распакуйте. Если
содержимое зашифровано (бинарный заголовок `contents.json`), сначала выполните
`decrypt` с подходящим ключом.

**Что такое 32-символьный ключ?**
Это мастер-ключ AES-256 пака. IV - это первые 16 байт ключа, а пофайловые ключи
находятся внутри зашифрованного `contents.json`.

**Это разрешено?**
Инструмент создан для исследователей безопасности, операторов серверов,
проверяющих собственные развёртывания, и авторов паков, восстанавливающих свои
ключи. Не используйте его для распространения чужого платного контента.

## Устранение неполадок

- **`featured` не показывает серверов или показывает другие серверы, чем в игре.**
  Mojang делит каталог по когорте PlayFab, привязанной к стабильному `.device_id`.
  Используйте **Settings > Reset featured cohort** (или удалите `.device_id`),
  чтобы получить новую.
- **Строка `[EVT]` падает при скачивании.** Для этого gathering сейчас нет
  активного venue; инструмент выводит понятное сообщение. Подождите или выберите
  другую запись.
- **Расшифровка выдаёт мусор.** Неверный ключ или пак был скачан не полностью.
  Перезапустите `download -v`, чтобы убедиться, что передача завершилась.
- **Авторизация постоянно снова запрашивает device code.** Каталог кэша токена
  (см. [Вход через Xbox Live](#вход-через-xbox-live)) недоступен для записи -
  обычное дело в песочницах - поэтому каждый запуск начинает авторизацию заново.

## Переменные окружения

| Переменная | Эффект |
| --- | --- |
| `NO_COLOR` | Отключает все ANSI-цвета ([no-color.org](https://no-color.org/)) |
| `BPT_DIAL_TIMEOUT` | Переопределяет таймаут подключения для `keys` / `download`, как Go duration (`5m`, `90s`). Ограничивает весь запуск, включая CDN-загрузки. |

## Безопасность

Инструмент авторизуется как обычный клиент Minecraft Bedrock, поэтому вам нужен
аккаунт Microsoft / Xbox Live. Кэшированные токены (`.xbox_token.json`,
`.mctoken.json`) и любой созданный вами `keys.json` - это настоящие учётные
данные: токены дают доступ к вашему аккаунту, а ключи расшифровывают платный
контент.

- Считайте каталог пользовательских настроек ОС (путь для каждой ОС см. в
  [Вход через Xbox Live](#вход-через-xbox-live)) секретным; не публикуйте и не
  передавайте его содержимое.
- Используйте инструмент на развёртываниях, которыми управляете, и на паках,
  которыми владеете. Проверяйте свои серверы, восстанавливайте свои ключи - но не
  распространяйте чужой платный контент.

## Участие в разработке

Issues и pull requests приветствуются - по любому нетривиальному изменению сначала
откройте issue для обсуждения. Перед отправкой выполните
`go build ./... && go test ./...` и используйте сообщения коммитов в стиле
conventional commits (`feat:`, `fix:`, `docs:`, ...). Полный набор проверок описан
в [CONTRIBUTING.md](CONTRIBUTING.md).

## Лицензия

Распространяется под лицензией MIT (SPDX: `MIT`). См. [LICENSE](LICENSE).
