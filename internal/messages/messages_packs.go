package messages

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

// Catalog for the pack pipeline subcommands: download, decrypt, encrypt.
// Keys are namespaced under packs.* so they never collide with catalogs
// contributed by other files. Only human-rendered progress, summary, and
// status lines live here - wrapped errors that humanize.go classifies are
// deliberately left in English at their call sites.
func init() {
	lang.Register(lang.English, map[string]string{
		// download: progress + connection
		"packs.download.connected":     "  Connected! %d packs, downloading...\n",
		"packs.download.dirLabel":      "C→S",
		"packs.download.dirLabelRecv":  "S→C",
		"packs.download.debugPacket":   "%s  [DEBUG] %s packet 0x%02x (%d bytes)\n",
		"packs.download.authenticated": "  Authenticated, loading packs...",
		"packs.download.progress":      "%s  Downloading: %.1f MB (%.0f KB/s)",
		"packs.download.warning":       "  Warning: %v\n",
		"packs.download.debugPack":     "  [DEBUG] Pack: %s v%s size=%d key=%q url=%q\n",
		"packs.download.cdnStart":      "%s  CDN download: %s v%s from %s\n",
		"packs.download.cdnFailed":     "  %s[ERR]%s CDN download failed: %v\n",
		"packs.download.cdnDownloaded": "  %s[CDN]%s Downloaded %.1f KB\n",
		"packs.download.openTmp":       "  %s[ERR]%s open tmp: %v\n",
		"packs.download.zipParse":      "  %s[ERR]%s zip parse: %v\n",
		"packs.download.extractFailed": "  %s[ERR]%s Extract failed: %v\n",
		"packs.download.okFiles":       "  %s[OK]%s  %-50s (%d files)\n",
		"packs.download.saveFailed":    "  %s[ERR]%s Save failed: %v\n",
		"packs.download.savedAs":       "  %s[OK]%s  Saved as %s (%.1f KB)\n",

		// download: usage
		"packs.download.usage": `Usage: bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]

Connect to a Minecraft Bedrock server, download all resource packs, and
extract them to disk. Also saves encryption keys.

Flags:
  -v, --verbose   Show all packet IDs for debugging
  -d, --decrypt   Decrypt the packs right after downloading (one step)

Output is grouped under <output-dir>/<server>/: one folder per pack
(Name_vVersion/), plus a keys.json with the encryption keys.

Without --decrypt the downloaded packs are still encrypted; turn them into
editable directories with: bedrock-pack-tools decrypt --all <keys.json> <output-dir>

Examples:
  bedrock-pack-tools download <server:port>
  bedrock-pack-tools download --decrypt <server:port> ./packs/`,

		// download: header banner
		"packs.download.bannerTop":    "\n  ┌─ Pack Downloader ─────────────────────────",
		"packs.download.bannerServer": "  │ Server: %s",
		"packs.download.bannerOutput": "  │ Output: %s",
		"packs.download.bannerBottom": "  └──────────────────────────────────────────",

		"packs.download.connecting": "Connecting to %s",

		// download: CDN-only completion path
		"packs.download.cdnComplete":    "\n  Downloaded %d/%d packs via CDN.\n",
		"packs.download.keysSaved":      "  Keys: %d -> %s\n",
		"packs.download.decryptFailed":  "\n  Decrypt step failed: %v\n",
		"packs.download.rerunDecrypt2":  "  Packs and keys are saved - rerun:  bedrock-pack-tools decrypt --all %s %s\n\n",
		"packs.download.rerunDecrypt1":  "  Packs and keys are saved - rerun:  bedrock-pack-tools decrypt --all %s %s\n",
		"packs.download.toDecrypt":      "  To decrypt:  bedrock-pack-tools decrypt --all %s %s\n",
		"packs.download.unencrypted":    "  Packs are unencrypted - ready to use, no decryption needed.",
		"packs.download.closedWithCdn":  "\n  Connection closed after %.1fs, but %d packs downloaded via CDN\n",
		"packs.download.closedWithKeys": "\n  Connection closed after %.1fs, but %d keys saved -> %s\n",
		"packs.download.noHandshake":    "  Packs could not be downloaded (server didn't complete handshake).",
		"packs.download.retry":          "  Retry:  bedrock-pack-tools download %s\n",

		// download: full-handshake summary
		"packs.download.summary":     "  Downloaded %d packs (%.1f MB) in %.1fs\n\n",
		"packs.download.extracting":  "  Extracting...",
		"packs.download.extractErr":  "  %s[ERR]%s  %s: %v\n",
		"packs.download.extractOk":   "  %s[OK]%s   %-50s (%d files)\n",
		"packs.download.savedCounts": "\n  Saved: %d/%d packs (%d encrypted, %d plain)\n",
		"packs.download.keysWarn":    "  Warning: could not save keys: %v\n",

		// extractZip - zip-slip skip warning (human text only; the
		// "  %s[WARN]%s " color/tag scaffolding stays at the call site).
		"packs.zipSlipSkipped": "zip-slip path skipped: ",

		// keyStore.merge - non-fatal key persistence failure
		"packs.warn.keysSaveFailed": "  Warning: could not save keys: %v\n",

		// decrypt: usage
		"packs.decrypt.usage": `Usage:
  bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]

Decrypt a single encrypted resource pack:
  bedrock-pack-tools decrypt ./my_packs/SomePack_v1.0.0 YOUR_32_CHAR_KEY

Batch-decrypt all packs matched by a keys.json file:
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/ ./decrypted/`,
		"packs.decrypt.usageAll":    "Usage: bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]",
		"packs.decrypt.usageSingle": "Usage: bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]",

		// decrypt: --all status
		"packs.decrypt.warn":     "  %s[WARN]%s %s - %v\n",
		"packs.decrypt.skip":     "  %s[SKIP]%s %s - no key for UUID %s\n",
		"packs.decrypt.noMatch":  "  No packs matched.",
		"packs.decrypt.jobError": "  %s[ERROR]%s %s: %v\n",
		"packs.decrypt.jobOk":    "  %s[OK]%s %s (%d decrypted, %d copied, %d errors)\n",
		"packs.decrypt.allDone":  "\n  Decrypted %d/%d packs\n  Location: %s\n",
		"packs.decrypt.escaped":  "    %s[WARN]%s path escapes output dir, skipped: %s\n",
		"packs.decrypt.fileErr":  "    %s[ERR]%s %s: %v\n",
		"packs.decrypt.copyErr":  "    %s[ERR]%s %s: %v\n",

		// decrypt: single-pack
		"packs.decrypt.packLabel":   "  Pack:   %s",
		"packs.decrypt.keyLabel":    "  Key:    %s",
		"packs.decrypt.outputLabel": "  Output: %s",
		"packs.decrypt.done":        "  Done! %d decrypted, %d copied, %d errors\n",

		// encrypt: usage
		"packs.encrypt.usage": `Usage:
  bedrock-pack-tools encrypt [--key-out PATH] <pack-dir> [key] [output.mcpack]

Encrypt a plain resource pack directory using AES-256-CFB8.
Produces a ready-to-use .mcpack file and a .mcpack.key file.

If the key is omitted, a random 32-character key is generated.
If the output is omitted, it defaults to <pack-name>.mcpack in the current directory.

Flags:
  --key-out PATH, -k PATH   Write the master key to PATH instead of
                            the default <output.mcpack>.key location.

Examples:
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567 ./out/MyPack.mcpack
  bedrock-pack-tools encrypt --key-out ~/keys/pack.key ./MyPack_v1.0.0/`,

		// encrypt: labels + summary
		"packs.encrypt.packLabel":    "  Pack:    %s",
		"packs.encrypt.keyLabel":     "  Key:     %s",
		"packs.encrypt.outputLabel":  "  Output:  %s",
		"packs.encrypt.keyfileLabel": "  Keyfile: %s",
		"packs.encrypt.done":         "  Done! %d encrypted, %d copied, %d errors\n",
		"packs.encrypt.outFile":      "  %s%s%s (%s)\n",
		"packs.encrypt.keyFile":      "  %s%s%s\n",
		"packs.encrypt.fileErr":      "    %s[ERR]%s %s: %v\n",
	})

	lang.Register(lang.Russian, map[string]string{
		// download: progress + connection
		"packs.download.connected":     "  Подключено! Паков: %d, загрузка...\n",
		"packs.download.dirLabel":      "C→S",
		"packs.download.dirLabelRecv":  "S→C",
		"packs.download.debugPacket":   "%s  [DEBUG] %s пакет 0x%02x (%d байт)\n",
		"packs.download.authenticated": "  Аутентифицирован, загрузка паков...",
		"packs.download.progress":      "%s  Загрузка: %.1f MB (%.0f KB/s)",
		"packs.download.warning":       "  Предупреждение: %v\n",
		"packs.download.debugPack":     "  [DEBUG] Пак: %s v%s size=%d key=%q url=%q\n",
		"packs.download.cdnStart":      "%s  Загрузка с CDN: %s v%s из %s\n",
		"packs.download.cdnFailed":     "  %s[ERR]%s ошибка загрузки с CDN: %v\n",
		"packs.download.cdnDownloaded": "  %s[CDN]%s Загружено %.1f KB\n",
		"packs.download.openTmp":       "  %s[ERR]%s не удалось открыть tmp: %v\n",
		"packs.download.zipParse":      "  %s[ERR]%s ошибка разбора zip: %v\n",
		"packs.download.extractFailed": "  %s[ERR]%s не удалось распаковать: %v\n",
		"packs.download.okFiles":       "  %s[OK]%s  %-50s (файлов: %d)\n",
		"packs.download.saveFailed":    "  %s[ERR]%s не удалось сохранить: %v\n",
		"packs.download.savedAs":       "  %s[OK]%s  Сохранено как %s (%.1f KB)\n",

		// download: usage
		"packs.download.usage": `Использование: bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]

Подключиться к серверу Minecraft Bedrock, скачать все ресурс-паки и
распаковать их на диск. Также сохраняет ключи шифрования.

Флаги:
  -v, --verbose   Показать все ID пакетов для отладки
  -d, --decrypt   Расшифровать паки сразу после загрузки (в один шаг)

Вывод группируется в <output-dir>/<server>/: по одной папке на пак
(Name_vVersion/) и файл keys.json с ключами шифрования.

Без --decrypt скачанные паки остаются зашифрованными; превратите их в
редактируемые каталоги командой: bedrock-pack-tools decrypt --all <keys.json> <output-dir>

Примеры:
  bedrock-pack-tools download <server:port>
  bedrock-pack-tools download --decrypt <server:port> ./packs/`,

		// download: header banner
		"packs.download.bannerTop":    "\n  ┌─ Загрузчик паков ─────────────────────────",
		"packs.download.bannerServer": "  │ Сервер: %s",
		"packs.download.bannerOutput": "  │ Выход:  %s",
		"packs.download.bannerBottom": "  └──────────────────────────────────────────",

		"packs.download.connecting": "Подключение к %s",

		// download: CDN-only completion path
		"packs.download.cdnComplete":    "\n  Скачано %d/%d паков через CDN.\n",
		"packs.download.keysSaved":      "  Ключи: %d -> %s\n",
		"packs.download.decryptFailed":  "\n  Расшифровка не удалась: %v\n",
		"packs.download.rerunDecrypt2":  "  Паки и ключи сохранены - запустите повторно:  bedrock-pack-tools decrypt --all %s %s\n\n",
		"packs.download.rerunDecrypt1":  "  Паки и ключи сохранены - запустите повторно:  bedrock-pack-tools decrypt --all %s %s\n",
		"packs.download.toDecrypt":      "  Для расшифровки:  bedrock-pack-tools decrypt --all %s %s\n",
		"packs.download.unencrypted":    "  Паки не зашифрованы - готовы к использованию, расшифровка не нужна.",
		"packs.download.closedWithCdn":  "\n  Соединение закрыто через %.1fs, но %d паков скачано через CDN\n",
		"packs.download.closedWithKeys": "\n  Соединение закрыто через %.1fs, но сохранено ключей: %d -> %s\n",
		"packs.download.noHandshake":    "  Не удалось скачать паки (сервер не завершил handshake).",
		"packs.download.retry":          "  Повтор:  bedrock-pack-tools download %s\n",

		// download: full-handshake summary
		"packs.download.summary":     "  Скачано паков: %d (%.1f MB) за %.1fs\n\n",
		"packs.download.extracting":  "  Распаковка...",
		"packs.download.extractErr":  "  %s[ERR]%s  %s: %v\n",
		"packs.download.extractOk":   "  %s[OK]%s   %-50s (файлов: %d)\n",
		"packs.download.savedCounts": "\n  Сохранено: %d/%d паков (зашифровано: %d, без шифрования: %d)\n",
		"packs.download.keysWarn":    "  Предупреждение: не удалось сохранить ключи: %v\n",

		// extractZip - zip-slip skip warning (human text only; the
		// "  %s[WARN]%s " color/tag scaffolding stays at the call site).
		"packs.zipSlipSkipped": "zip-slip пропущен: ",

		// keyStore.merge - non-fatal key persistence failure
		"packs.warn.keysSaveFailed": "  Предупреждение: не удалось сохранить ключи: %v\n",

		// decrypt: usage
		"packs.decrypt.usage": `Использование:
  bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]

Расшифровать один зашифрованный ресурс-пак:
  bedrock-pack-tools decrypt ./my_packs/SomePack_v1.0.0 YOUR_32_CHAR_KEY

Пакетно расшифровать все паки, сопоставленные файлом keys.json:
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/ ./decrypted/`,
		"packs.decrypt.usageAll":    "Использование: bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]",
		"packs.decrypt.usageSingle": "Использование: bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]",

		// decrypt: --all status
		"packs.decrypt.warn":     "  %s[WARN]%s %s - %v\n",
		"packs.decrypt.skip":     "  %s[SKIP]%s %s - нет ключа для UUID %s\n",
		"packs.decrypt.noMatch":  "  Подходящих паков нет.",
		"packs.decrypt.jobError": "  %s[ERROR]%s %s: %v\n",
		"packs.decrypt.jobOk":    "  %s[OK]%s %s (расшифровано: %d, скопировано: %d, ошибок: %d)\n",
		"packs.decrypt.allDone":  "\n  Расшифровано %d/%d паков\n  Расположение: %s\n",
		"packs.decrypt.escaped":  "    %s[WARN]%s путь выходит за пределы выходного каталога, пропущено: %s\n",
		"packs.decrypt.fileErr":  "    %s[ERR]%s %s: %v\n",
		"packs.decrypt.copyErr":  "    %s[ERR]%s %s: %v\n",

		// decrypt: single-pack
		"packs.decrypt.packLabel":   "  Пак:    %s",
		"packs.decrypt.keyLabel":    "  Ключ:   %s",
		"packs.decrypt.outputLabel": "  Выход:  %s",
		"packs.decrypt.done":        "  Готово! расшифровано: %d, скопировано: %d, ошибок: %d\n",

		// encrypt: usage
		"packs.encrypt.usage": `Использование:
  bedrock-pack-tools encrypt [--key-out PATH] <pack-dir> [key] [output.mcpack]

Зашифровать каталог обычного ресурс-пака с помощью AES-256-CFB8.
Создает готовый к использованию файл .mcpack и файл .mcpack.key.

Если ключ опущен, генерируется случайный ключ из 32 символов.
Если выход опущен, по умолчанию используется <pack-name>.mcpack в текущем каталоге.

Флаги:
  --key-out PATH, -k PATH   Записать мастер-ключ в PATH вместо
                            расположения по умолчанию <output.mcpack>.key.

Примеры:
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567 ./out/MyPack.mcpack
  bedrock-pack-tools encrypt --key-out ~/keys/pack.key ./MyPack_v1.0.0/`,

		// encrypt: labels + summary
		"packs.encrypt.packLabel":    "  Пак:        %s",
		"packs.encrypt.keyLabel":     "  Ключ:       %s",
		"packs.encrypt.outputLabel":  "  Выход:      %s",
		"packs.encrypt.keyfileLabel": "  Файл ключа: %s",
		"packs.encrypt.done":         "  Готово! зашифровано: %d, скопировано: %d, ошибок: %d\n",
		"packs.encrypt.outFile":      "  %s%s%s (%s)\n",
		"packs.encrypt.keyFile":      "  %s%s%s\n",
		"packs.encrypt.fileErr":      "    %s[ERR]%s %s: %v\n",
	})
}
