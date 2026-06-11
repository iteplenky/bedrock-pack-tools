package main

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

// This file owns two domain prefixes: featured.* (the `featured`
// subcommand, featured.go) and keys.* (the `keys` subcommand, keys.go).
// Both source files are migrated together, so registering their catalogs
// from one init keeps the EN/RU pairs side by side. The prefixes are
// unique, so this never collides with catalogs other files register.

func init() {
	lang.Register(lang.English, map[string]string{
		// featured.go - dispatch + usage
		"featured.download.needindex": "featured download requires an index - run 'bedrock-pack-tools featured' to see the list",
		"featured.usage": `Usage:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download <index> [output-dir]

List the Featured Servers and Live Events surfaced by Minecraft's
client-discovery service, then optionally download a chosen entry.

Requires Xbox Live authentication (token cached on first use).

Examples:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download 1
  bedrock-pack-tools featured download 1 ./packs/`,

		// featured.go - list view header + spinners + summary
		"featured.list.header.title":  "\n  ┌─ Featured Servers ────────────────────────",
		"featured.list.header.source": "  │ Source: Minecraft gatherings API",
		"featured.list.header.rule":   "  └──────────────────────────────────────────",
		"featured.list.spinner.fetch": "Fetching catalog",
		"featured.list.empty":         "  No featured servers returned by the API.",
		"featured.list.spinner.ping":  "Pinging %d servers",
		"featured.list.todownload":    "\n  To download:  bedrock-pack-tools featured download <index> [output-dir]",

		// featured.go - download flow
		"featured.download.spinner.fetch": "Fetching catalog",
		"featured.download.badindex":      "invalid index %q: must be a positive integer",
		"featured.download.outofrange":    "index %d out of range (have %d featured servers)",
		"featured.download.resolved":      "\n  [->] %s  ->  %s\n",

		// featured.go - resolve errors
		"featured.resolve.gathering.forbidden":  "%q is not joinable by this account (Mojang returned forbidden)",
		"featured.resolve.gathering.fail":       "resolve gathering %q: %w",
		"featured.resolve.experience.offline":   "%q has no active venue right now (the slot is listed but not joinable from outside the official client)",
		"featured.resolve.experience.forbidden": "%q is not joinable by this account (Mojang returned forbidden - it may be region-locked or only joinable from the official client)",
		"featured.resolve.experience.fail":      "resolve experience %q: %w",
		"featured.resolve.unknownkind":          "unknown featured kind for %q",

		// featured.go - non-fatal live-events warning
		"featured.liveevents.warn": "  Warning: could not fetch live events: %v\n",

		// featured.go - address column placeholders
		"featured.addr.liveevent":  "(live event)",
		"featured.addr.experience": "(experience-join)",
		"featured.addr.none":       "(no address)",

		// featured.go - status labels
		"featured.status.resolve":     "resolve on download",
		"featured.status.offline":     "offline",
		"featured.status.online":      "online",
		"featured.status.online.one":  "online 1 player",
		"featured.status.online.many": "online %s players",

		// keys.go - usage
		"keys.usage": `Usage: bedrock-pack-tools keys <server:port> [output.json]

Connect to a Minecraft Bedrock server and extract resource pack encryption
keys. Authenticates via Xbox Live device code flow (token cached locally).

Examples:
  bedrock-pack-tools keys <server:port>
  bedrock-pack-tools keys <server:port> server_keys.json`,

		// keys.go - per-pack skip line
		"keys.pack.skipped": "%s  Pack %d/%d: %s v%s (skipped)",

		// keys.go - resource-packs parse warning
		"keys.warn": "  Warning: %v\n",

		// keys.go - header box
		"keys.header.title":  "\n  ┌─ Pack Key Dumper ─────────────────────────",
		"keys.header.server": "  │ Server: ",
		"keys.header.output": "  │ Output: ",
		"keys.header.rule":   "  └──────────────────────────────────────────",

		// keys.go - connect spinner
		"keys.spinner.connecting": "Connecting to ",

		// keys.go - partial-result path (connection failed but keys captured)
		"keys.partial.captured": "%s  Keys captured in %.1fs (%d packs on server)\n\n",
		"keys.partial.saved":    "\n  Saved %d keys -> %s\n\n",
		"keys.connect.failed":   "connection to %s failed: %w",

		// keys.go - success path
		"keys.connected": "%s  Connected! (%.1fs)\n\n",
		"keys.total":     "\n  Total: %d packs (%d encrypted)\n",
		"keys.saved":     "  Saved %d keys -> %s\n\n",
		"keys.none":      "\n  No encryption keys found - nothing written to ",
		"keys.none.tail": ".",

		// keys.go - per-key listing
		"keys.entry.head": "  %s[ENC]%s %s v%s \"%s\"\n",
		"keys.entry.key":  "        KEY: %s%s%s\n",
	})

	lang.Register(lang.Russian, map[string]string{
		// featured.go - dispatch + usage
		"featured.download.needindex": "для `featured download` нужен индекс - запустите 'bedrock-pack-tools featured', чтобы увидеть список",
		"featured.usage": `Использование:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download <index> [output-dir]

Показывает Featured Servers и Live Events из сервиса client-discovery
Minecraft, после чего по желанию загружает выбранную запись.

Требуется аутентификация Xbox Live (токен кэшируется при первом запуске).

Примеры:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download 1
  bedrock-pack-tools featured download 1 ./packs/`,

		// featured.go - list view header + spinners + summary
		"featured.list.header.title":  "\n  ┌─ Featured Servers ────────────────────────",
		"featured.list.header.source": "  │ Источник: Minecraft gatherings API",
		"featured.list.header.rule":   "  └──────────────────────────────────────────",
		"featured.list.spinner.fetch": "Загрузка каталога",
		"featured.list.empty":         "  API не вернул ни одного Featured-сервера.",
		"featured.list.spinner.ping":  "Пинг серверов: %d",
		"featured.list.todownload":    "\n  Для загрузки:  bedrock-pack-tools featured download <index> [output-dir]",

		// featured.go - download flow
		"featured.download.spinner.fetch": "Загрузка каталога",
		"featured.download.badindex":      "некорректный индекс %q: нужно положительное целое число",
		"featured.download.outofrange":    "индекс %d вне диапазона (всего Featured-серверов: %d)",
		"featured.download.resolved":      "\n  [->] %s  ->  %s\n",

		// featured.go - resolve errors
		"featured.resolve.gathering.forbidden":  "%q недоступен для этого аккаунта (Mojang запретил подключение)",
		"featured.resolve.gathering.fail":       "не удалось определить адрес gathering %q: %w",
		"featured.resolve.experience.offline":   "у %q сейчас нет активной площадки (слот есть в списке, но подключиться можно только из официального клиента)",
		"featured.resolve.experience.forbidden": "%q недоступен для этого аккаунта (Mojang запретил подключение - возможно, регион заблокирован или подключиться можно только из официального клиента)",
		"featured.resolve.experience.fail":      "не удалось определить адрес experience %q: %w",
		"featured.resolve.unknownkind":          "неизвестный тип Featured-записи для %q",

		// featured.go - non-fatal live-events warning
		"featured.liveevents.warn": "  Предупреждение: не удалось загрузить live events: %v\n",

		// featured.go - address column placeholders
		"featured.addr.liveevent":  "(live event)",
		"featured.addr.experience": "(experience-join)",
		"featured.addr.none":       "(нет адреса)",

		// featured.go - status labels
		"featured.status.resolve":     "определение адреса при загрузке",
		"featured.status.offline":     "офлайн",
		"featured.status.online":      "онлайн",
		"featured.status.online.one":  "онлайн, 1 игрок",
		"featured.status.online.many": "онлайн, игроков: %s",

		// keys.go - usage
		"keys.usage": `Использование: bedrock-pack-tools keys <server:port> [output.json]

Подключается к серверу Minecraft Bedrock и извлекает ключи шифрования
ресурс-паков. Аутентификация через вход по коду устройства Xbox Live
(токен кэшируется локально).

Примеры:
  bedrock-pack-tools keys <server:port>
  bedrock-pack-tools keys <server:port> server_keys.json`,

		// keys.go - per-pack skip line
		"keys.pack.skipped": "%s  Пак %d/%d: %s v%s (пропущен)",

		// keys.go - resource-packs parse warning
		"keys.warn": "  Предупреждение: %v\n",

		// keys.go - header box
		"keys.header.title":  "\n  ┌─ Дамп ключей паков ───────────────────────",
		"keys.header.server": "  │ Сервер: ",
		"keys.header.output": "  │ Вывод: ",
		"keys.header.rule":   "  └──────────────────────────────────────────",

		// keys.go - connect spinner
		"keys.spinner.connecting": "Подключение к ",

		// keys.go - partial-result path (connection failed but keys captured)
		"keys.partial.captured": "%s  Ключи получены за %.1fs (паков на сервере: %d)\n\n",
		"keys.partial.saved":    "\n  Сохранено ключей: %d -> %s\n\n",
		// NOT translated: humanize.go classifies game-server failures by
		// substring-matching this exact `connection to <server> failed:`
		// wrap (reGameServer). Translating it breaks the protocol/kick/
		// timeout diagnostics for the keys command. Keep identical to EN.
		"keys.connect.failed": "connection to %s failed: %w",

		// keys.go - success path
		"keys.connected": "%s  Подключено! (%.1fs)\n\n",
		"keys.total":     "\n  Всего: %d паков (зашифровано %d)\n",
		"keys.saved":     "  Сохранено ключей: %d -> %s\n\n",
		"keys.none":      "\n  Ключи шифрования не найдены - в файл ничего не записано: ",
		"keys.none.tail": ".",

		// keys.go - per-key listing
		"keys.entry.head": "  %s[ENC]%s %s v%s \"%s\"\n",
		"keys.entry.key":  "        KEY: %s%s%s\n",
	})
}
