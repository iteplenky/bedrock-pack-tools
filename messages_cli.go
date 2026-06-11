package main

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

func init() {
	lang.Register(lang.English, map[string]string{
		"usage.dialtimeout.warning": "  Warning: %s=%q is not a valid duration, using %s\n",
		"usage.unknown.command":     "Unknown command: %s\n\n",
		"usage.help": `bedrock-pack-tools - dump, download, decrypt & encrypt Minecraft Bedrock resource packs

Usage:
  bedrock-pack-tools                                  (no command: interactive menu)
  bedrock-pack-tools keys     <server:port> [output.json]
  bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]
  bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools encrypt  [--key-out PATH] <pack-dir> [key] [output.mcpack]
  bedrock-pack-tools featured [download <index> [output-dir]]
  bedrock-pack-tools login
  bedrock-pack-tools logout
  bedrock-pack-tools version

Environment:
  BPT_DIAL_TIMEOUT  Override the keys/download dial timeout (e.g. "5m", "90s").
                    Useful for slow servers or long ResourcePacksInfo waits.

Commands:
  keys      Connect to a Bedrock server and extract resource pack encryption keys.
            Requires Xbox Live authentication (device code flow, token cached).

  download  Connect to a Bedrock server, download all resource packs, and extract
            them to disk. Also saves encryption keys. Packs are encrypted on disk;
            add --decrypt to decrypt in the same step, or run 'decrypt' afterwards.

  decrypt   Decrypt an encrypted resource pack using a 32-character AES key,
            or batch-decrypt all packs matched by a keys.json file.

  encrypt   Encrypt a plain resource pack into a ready-to-use .mcpack file
            with a .mcpack.key beside it. Uses AES-256-CFB8 with per-file keys.
            If no key is provided, one is generated automatically.

  featured  List the Featured Servers and Live Events from Minecraft's
            client-discovery API and optionally download one by index.

  login     Sign in via Xbox Live (device code flow) and cache the token.
  logout    Remove the cached Xbox + franchise tokens.

  version   Show version information.

Run "bedrock-pack-tools <command>" with no arguments for command-specific help.`,
	})

	lang.Register(lang.Russian, map[string]string{
		"usage.dialtimeout.warning": "  Предупреждение: %s=%q - неверная длительность, применяется %s\n",
		"usage.unknown.command":     "Неизвестная команда: %s\n\n",
		"usage.help": `bedrock-pack-tools - дамп, загрузка, расшифровка и шифрование ресурс-паков Minecraft Bedrock

Использование:
  bedrock-pack-tools                                  (без команды: интерактивное меню)
  bedrock-pack-tools keys     <server:port> [output.json]
  bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]
  bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools encrypt  [--key-out PATH] <pack-dir> [key] [output.mcpack]
  bedrock-pack-tools featured [download <index> [output-dir]]
  bedrock-pack-tools login
  bedrock-pack-tools logout
  bedrock-pack-tools version

Переменные окружения:
  BPT_DIAL_TIMEOUT  Переопределяет таймаут подключения для keys/download (например, "5m", "90s").
                    Полезно для медленных серверов или долгого ожидания ResourcePacksInfo.

Команды:
  keys      Подключиться к серверу Bedrock и извлечь ключи шифрования ресурс-паков.
            Требует аутентификации Xbox Live (device code flow, токен кэшируется).

  download  Подключиться к серверу Bedrock, загрузить все ресурс-паки и распаковать
            их на диск. Также сохраняет ключи шифрования. Паки на диске зашифрованы;
            добавьте --decrypt, чтобы расшифровать сразу, или запустите 'decrypt' позже.

  decrypt   Расшифровать зашифрованный ресурс-пак 32-символьным ключом AES
            или пакетно расшифровать все паки, сопоставленные файлом keys.json.

  encrypt   Зашифровать обычный ресурс-пак в готовый к использованию файл .mcpack
            с файлом .mcpack.key рядом. Использует AES-256-CFB8 с пофайловыми ключами.
            Если ключ не указан, он генерируется автоматически.

  featured  Показать Featured Servers и Live Events из client-discovery API
            Minecraft и при необходимости загрузить один из них по индексу.

  login     Войти через Xbox Live (device code flow) и кэшировать токен.
  logout    Удалить кэшированные токены Xbox и franchise.

  version   Показать информацию о версии.

Запустите "bedrock-pack-tools <command>" без аргументов для справки по конкретной команде.`,
	})
}
