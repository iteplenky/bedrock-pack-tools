package messages

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

func init() {
	lang.Register(lang.English, map[string]string{
		// Main menu sections (label + description).
		"tui.section.featured.label": "Featured servers",
		"tui.section.featured.desc":  "Browse Mojang's live catalog and pick one or more.",
		"tui.section.address.label":  "Enter a server address",
		"tui.section.address.desc":   "Type any host:port, e.g. play.example.net:19132.",
		"tui.section.saved.label":    "Saved servers",
		"tui.section.saved.desc":     "Addresses you saved for quick re-use.",
		"tui.section.recent.label":   "Recent addresses",
		"tui.section.recent.desc":    "Addresses you entered recently, with their last result.",
		"tui.section.decrypt.label":  "Decrypt packs",
		"tui.section.decrypt.desc":   "Decrypt packs you've downloaded - or fetch what's missing.",
		"tui.section.encrypt.label":  "Encrypt a pack",
		"tui.section.encrypt.desc":   "Package a local pack folder into a .mcpack + .mcpack.key.",
		"tui.section.settings.label": "Settings",
		"tui.section.settings.desc":  "Sign in or out, clear saved data, or reset the featured cohort.",

		// Confirmation prompts.
		"tui.confirm.logout":         "Forget the cached sign-in?",
		"tui.confirm.clearAddrs":     "Clear saved and recent addresses?",
		"tui.confirm.clearDownloads": "Forget download history? (packs on disk are kept)",
		"tui.confirm.resetCohort":    "Reset the featured cohort? Reopen Featured to see the new list.",

		// Settings rows (label + description).
		"tui.settings.language.label":       "Language: %s",
		"tui.settings.language.desc":        "Switch the interface language.",
		"tui.settings.clearAddrs.label":     "Clear saved and recent",
		"tui.settings.clearAddrs.desc":      "Forget your saved servers and recent addresses.",
		"tui.settings.clearDownloads.label": "Clear download history",
		"tui.settings.clearDownloads.desc":  "Forget where past downloads landed (packs on disk are kept).",
		"tui.settings.resetCohort.label":    "Reset featured cohort",
		"tui.settings.resetCohort.desc":     "Roll a new device id - reopen Featured for a fresh list.",
		"tui.settings.signIn.label":         "Sign in",
		"tui.settings.signIn.desc":          "Run Xbox device sign-in - a URL and code will appear.",
		"tui.settings.signOut.label":        "Sign out",
		"tui.settings.signOut.desc":         "Forget the cached Xbox token and franchise session.",

		// Transient notes (confirmations / errors shown inline).
		"tui.note.resolved":            "resolved %s",
		"tui.note.noResolved":          "no addresses resolved right now",
		"tui.note.signedIn":            "signed in",
		"tui.note.signInIncomplete":    "sign-in did not complete",
		"tui.note.forgot":              "forgot %s",
		"tui.note.saved":               "saved %s",
		"tui.note.savedAddr":           "saved %s",
		"tui.note.languageChanged":     "Language set to %s",
		"tui.note.signInFirstFeatured": "sign in first to browse featured servers",
		"tui.note.signInFirstRun":      "sign in first - Settings > Sign in",
		"tui.note.logoutFailed":        "logout failed: %s",
		"tui.note.signedOut":           "signed out",
		"tui.note.alreadySignedOut":    "already signed out",
		"tui.note.clearedAddrs":        "cleared saved and recent",
		"tui.note.clearedDownloads":    "cleared download history",
		"tui.note.resetFailed":         "reset failed: %s",
		"tui.note.cohortReset":         "cohort reset - reopen Featured to refresh",
		"tui.note.noAddrEntry":         "no saved address for this entry - press d to forget it",
		"tui.note.nothingToDecrypt":    "nothing to decrypt - the packs are gone, press g to download them again",

		// Status / progress lines.
		"tui.status.starting":   "Starting...",
		"tui.status.resolving":  "Resolving address...",
		"tui.status.canceling":  "[canceling]",
		"tui.status.pausedWith": "[paused] %s",
		"tui.status.paused":     "[paused]",

		// Live running-log line prefixes (concatenated with child output).
		"tui.log.err":     "[err] %s",
		"tui.log.partial": "[partial] %s",

		// Breadcrumb trail segments.
		"tui.crumb.home":         "Home",
		"tui.crumb.loading":      "Loading",
		"tui.crumb.chooseAction": "Choose an action",
		"tui.crumb.working":      "Working",

		// Loading screen.
		"tui.loading.featured": "Loading featured servers...",

		// Empty-list messages.
		"tui.empty.saved":   "Nothing saved yet - save an address from Recent or the address screen.",
		"tui.empty.recent":  "No recent addresses yet - enter one from the address screen.",
		"tui.empty.decrypt": "Nothing downloaded yet - download a server first, then come back to decrypt.",

		// Menu header tagline.
		"tui.header.tagline1": "Dump, download, and decrypt",
		"tui.header.tagline2": "Minecraft Bedrock resource packs",

		// Menu / load error banner.
		"tui.error.loadFeatured": "Could not load featured servers: %s",

		// Featured view.
		"tui.featured.resolving": "Resolving addresses...",

		// featuredHelp - highlighted-row help.
		"tui.featuredHelp.direct":     "Direct address: %s",
		"tui.featuredHelp.liveEvent":  "Live event - press ^r to resolve its address (or it resolves on download).",
		"tui.featuredHelp.experience": "Experience server - press ^r to resolve its address (or it resolves on download).",
		"tui.featuredHelp.none":       "No public address for this entry.",

		// Address screen.
		"tui.address.label":   "Server address: ",
		"tui.address.example": "Example: play.example.net:19132 or 1.2.3.4:19132",

		// Encrypt screen.
		"tui.encrypt.label":   "Pack directory: ",
		"tui.encrypt.example": "Point at a folder with a manifest.json, e.g. ./MyPack_v1.0.0",

		// Settings screen status.
		"tui.settings.signedIn":    "Signed in",
		"tui.settings.notSignedIn": "Not signed in",
		"tui.settings.config":      "config: %s",

		// Action picker.
		"tui.action.for":                   "Action for %s",
		"tui.action.download.label":        "Download packs",
		"tui.action.download.desc":         "Save the keys file and the encrypted packs to this folder.",
		"tui.action.downloadDecrypt.label": "Download + decrypt",
		"tui.action.downloadDecrypt.desc":  "Download, then turn every pack into a ready-to-edit folder.",
		"tui.action.keys.label":            "Keys only",
		"tui.action.keys.desc":             "Just dump the AES content keys - no packs downloaded.",

		// Running screen.
		"tui.running.job": "job %d/%d",

		// Done screen.
		"tui.done.canceled":         "Canceled",
		"tui.done.done":             "Done",
		"tui.done.err":              "[err]",
		"tui.done.partial":          "[partial]",
		"tui.done.ok":               "[ok]",
		"tui.done.decryptedTo":      "decrypted -> ",
		"tui.done.succeeded":        "%d/%d succeeded",
		"tui.done.partialSummary":   "%d partial - output landed but the run did not fully finish",
		"tui.done.skippedSummary":   "%d skipped - needed a download first",
		"tui.done.encryptWritten":   ".mcpack + .mcpack.key written to the current directory",
		"tui.done.keysSaved":        "keys saved to the current directory",
		"tui.done.downloadsCurrent": "downloaded to the current directory",

		// Plural helpers.
		"tui.plural.server.one":  "%d server",
		"tui.plural.server.few":  "%d servers",
		"tui.plural.server.many": "%d servers",
		"tui.plural.pack.one":    "%d pack",
		"tui.plural.pack.few":    "%d packs",
		"tui.plural.pack.many":   "%d packs",
		"tui.plural.addr.one":    "%d address",
		"tui.plural.addr.few":    "%d addresses",
		"tui.plural.addr.many":   "%d addresses",

		// Validation errors (encrypt pack-dir + address field).
		"tui.validate.enterPath":  "enter a path to a pack folder",
		"tui.validate.notFolder":  "not a folder - point at a resource-pack directory",
		"tui.validate.noManifest": "no manifest.json there - point at a resource-pack directory",
		"tui.validate.expectAddr": "expected host:port, e.g. play.example.net:19132",

		// Recent-row status badge.
		"tui.recentStatus.ok":     "ok",
		"tui.recentStatus.failed": "failed",

		// Decrypt section.
		"tui.decrypt.badge.keys":      "keys",
		"tui.decrypt.badge.decrypted": "decrypted",
		"tui.decryptHelp.reDecrypt":   "Already decrypted - press enter to re-decrypt %s into a decrypted/<server>/ folder beside the packs.",
		"tui.decryptHelp.decrypt":     "Decrypt %s - output lands in a decrypted/<server>/ folder beside the packs.",
		"tui.decryptHelp.packsGone":   "Keys are here but the packs are gone - press g to download and decrypt them again.",
		"tui.decryptHelp.noAddr":      "Keys are here but the packs are gone, and no saved address to re-download.",

		// Featured filter line + empty states.
		"tui.featuredList.filter":    "filter: %s",
		"tui.featuredList.selHidden": "%d selected (%d hidden by filter)",
		"tui.featuredList.empty":     "No featured servers right now - try again later.",
		"tui.featuredList.noMatch":   "No servers match your filter.",

		// Relative-time labels for the recent-downloads list (ageLabel).
		"tui.age.justNow":    "just now",
		"tui.age.minutesAgo": "%dm ago",
		"tui.age.hoursAgo":   "%dh ago",
		"tui.age.daysAgo":    "%dd ago",

		// Hint-bar labels.
		"tui.hint.cancel":          "cancel",
		"tui.hint.move":            "move",
		"tui.hint.open":            "open",
		"tui.hint.quit":            "quit",
		"tui.hint.back":            "back",
		"tui.hint.select":          "select",
		"tui.hint.continue":        "continue",
		"tui.hint.resolveIPs":      "resolve IPs",
		"tui.hint.filter":          "filter",
		"tui.hint.clearFilter":     "clear filter",
		"tui.hint.save":            "save",
		"tui.hint.forget":          "forget",
		"tui.hint.moveCaret":       "move caret",
		"tui.hint.encrypt":         "encrypt",
		"tui.hint.yes":             "yes",
		"tui.hint.start":           "start",
		"tui.hint.pause":           "pause",
		"tui.hint.resume":          "resume",
		"tui.hint.decrypt":         "decrypt",
		"tui.hint.downloadDecrypt": "download+decrypt",
		"tui.hint.backToMenu":      "back to menu",
	})

	lang.Register(lang.Russian, map[string]string{
		// Main menu sections (label + description).
		"tui.section.featured.label": "Featured серверы",
		"tui.section.featured.desc":  "Открыть каталог Mojang и выбрать один или несколько.",
		"tui.section.address.label":  "Ввести адрес сервера",
		"tui.section.address.desc":   "Введите любой host:port, например play.example.net:19132.",
		"tui.section.saved.label":    "Сохранённые серверы",
		"tui.section.saved.desc":     "Адреса, сохранённые для быстрого повторного использования.",
		"tui.section.recent.label":   "Недавние адреса",
		"tui.section.recent.desc":    "Адреса, введённые недавно, с их последним результатом.",
		"tui.section.decrypt.label":  "Расшифровка паков",
		"tui.section.decrypt.desc":   "Расшифровать загруженные паки - или докачать отсутствующие.",
		"tui.section.encrypt.label":  "Зашифровать пак",
		"tui.section.encrypt.desc":   "Упаковать локальную папку пака в .mcpack + .mcpack.key.",
		"tui.section.settings.label": "Настройки",
		"tui.section.settings.desc":  "Войти или выйти, очистить сохранённые данные или сбросить featured-когорту.",

		// Confirmation prompts.
		"tui.confirm.logout":         "Очистить сохранённый вход?",
		"tui.confirm.clearAddrs":     "Очистить сохранённые и недавние адреса?",
		"tui.confirm.clearDownloads": "Забыть историю загрузок? (паки на диске останутся)",
		"tui.confirm.resetCohort":    "Сбросить featured-когорту? Откройте Featured заново, чтобы увидеть новый список.",

		// Settings rows (label + description).
		"tui.settings.language.label":       "Язык: %s",
		"tui.settings.language.desc":        "Переключить язык интерфейса.",
		"tui.settings.clearAddrs.label":     "Очистить сохранённые и недавние",
		"tui.settings.clearAddrs.desc":      "Забыть сохранённые серверы и недавние адреса.",
		"tui.settings.clearDownloads.label": "Очистить историю загрузок",
		"tui.settings.clearDownloads.desc":  "Забыть, куда легли прошлые загрузки (паки на диске останутся).",
		"tui.settings.resetCohort.label":    "Сбросить featured-когорту",
		"tui.settings.resetCohort.desc":     "Выпустить новый device id - откройте Featured заново для нового списка.",
		"tui.settings.signIn.label":         "Войти",
		"tui.settings.signIn.desc":          "Запустить вход по коду устройства - появятся URL и код.",
		"tui.settings.signOut.label":        "Выйти",
		"tui.settings.signOut.desc":         "Забыть сохранённый токен Xbox и franchise-сессию.",

		// Transient notes (confirmations / errors shown inline).
		"tui.note.resolved":            "определено: %s",
		"tui.note.noResolved":          "сейчас ни один адрес не определён",
		"tui.note.signedIn":            "вход выполнен",
		"tui.note.signInIncomplete":    "вход не завершён",
		"tui.note.forgot":              "забыто %s",
		"tui.note.saved":               "сохранено %s",
		"tui.note.savedAddr":           "сохранено %s",
		"tui.note.languageChanged":     "Язык переключён на %s",
		"tui.note.signInFirstFeatured": "сначала войдите, чтобы просматривать featured-серверы",
		"tui.note.signInFirstRun":      "сначала войдите - Настройки > Войти",
		"tui.note.logoutFailed":        "не удалось выйти: %s",
		"tui.note.signedOut":           "выход выполнен",
		"tui.note.alreadySignedOut":    "выход уже выполнен",
		"tui.note.clearedAddrs":        "сохранённые и недавние очищены",
		"tui.note.clearedDownloads":    "история загрузок очищена",
		"tui.note.resetFailed":         "не удалось сбросить: %s",
		"tui.note.cohortReset":         "когорта сброшена - откройте Featured заново для обновления",
		"tui.note.noAddrEntry":         "для этой записи нет сохранённого адреса - нажмите d, чтобы её забыть",
		"tui.note.nothingToDecrypt":    "нечего расшифровать - паков нет, нажмите g, чтобы скачать их снова",

		// Status / progress lines.
		"tui.status.starting":   "Запуск...",
		"tui.status.resolving":  "Определение адреса...",
		"tui.status.canceling":  "[отмена]",
		"tui.status.pausedWith": "[пауза] %s",
		"tui.status.paused":     "[пауза]",

		// Live running-log line prefixes (concatenated with child output).
		"tui.log.err":     "[ошибка] %s",
		"tui.log.partial": "[частично] %s",

		// Breadcrumb trail segments.
		"tui.crumb.home":         "Главная",
		"tui.crumb.loading":      "Загрузка",
		"tui.crumb.chooseAction": "Выбор действия",
		"tui.crumb.working":      "Выполнение",

		// Loading screen.
		"tui.loading.featured": "Загрузка featured-серверов...",

		// Empty-list messages.
		"tui.empty.saved":   "Пока ничего не сохранено - сохраните адрес из недавних или экрана адреса.",
		"tui.empty.recent":  "Пока нет недавних адресов - введите один на экране адреса.",
		"tui.empty.decrypt": "Пока ничего не загружено - сначала скачайте сервер, потом вернитесь к расшифровке.",

		// Menu header tagline.
		"tui.header.tagline1": "Извлечение, загрузка и расшифровка",
		"tui.header.tagline2": "ресурс-паков Minecraft Bedrock",

		// Menu / load error banner.
		"tui.error.loadFeatured": "Не удалось загрузить featured-серверы: %s",

		// Featured view.
		"tui.featured.resolving": "Определение адресов...",

		// featuredHelp - highlighted-row help.
		"tui.featuredHelp.direct":     "Прямой адрес: %s",
		"tui.featuredHelp.liveEvent":  "Live event - нажмите ^r, чтобы определить адрес (или он определится при загрузке).",
		"tui.featuredHelp.experience": "Experience-сервер - нажмите ^r, чтобы определить адрес (или он определится при загрузке).",
		"tui.featuredHelp.none":       "У этой записи нет публичного адреса.",

		// Address screen.
		"tui.address.label":   "Адрес сервера: ",
		"tui.address.example": "Пример: play.example.net:19132 или 1.2.3.4:19132",

		// Encrypt screen.
		"tui.encrypt.label":   "Папка пака: ",
		"tui.encrypt.example": "Укажите папку с manifest.json, например ./MyPack_v1.0.0",

		// Settings screen status.
		"tui.settings.signedIn":    "Вход выполнен",
		"tui.settings.notSignedIn": "Вход не выполнен",
		"tui.settings.config":      "config: %s",

		// Action picker.
		"tui.action.for":                   "Действие · %s",
		"tui.action.download.label":        "Скачать паки",
		"tui.action.download.desc":         "Сохранить файл ключей и зашифрованные паки в этот каталог.",
		"tui.action.downloadDecrypt.label": "Скачать + расшифровать",
		"tui.action.downloadDecrypt.desc":  "Скачать и превратить каждый пак в готовую к редактированию папку.",
		"tui.action.keys.label":            "Только ключи",
		"tui.action.keys.desc":             "Только выгрузить AES-ключи контента - паки не загружаются.",

		// Running screen.
		"tui.running.job": "задача %d/%d",

		// Done screen.
		"tui.done.canceled":         "Отменено",
		"tui.done.done":             "Готово",
		"tui.done.err":              "[ошибка]",
		"tui.done.partial":          "[частично]",
		"tui.done.ok":               "[ок]",
		"tui.done.decryptedTo":      "расшифровано -> ",
		"tui.done.succeeded":        "успешно: %d/%d",
		"tui.done.partialSummary":   "частично: %d - результат записан, но выполнение завершилось неполно",
		"tui.done.skippedSummary":   "пропущено: %d - сначала нужна загрузка",
		"tui.done.encryptWritten":   ".mcpack + .mcpack.key записаны в текущий каталог",
		"tui.done.keysSaved":        "ключи сохранены в текущий каталог",
		"tui.done.downloadsCurrent": "загружено в текущий каталог",

		// Plural helpers.
		"tui.plural.server.one":  "%d сервер",
		"tui.plural.server.few":  "%d сервера",
		"tui.plural.server.many": "%d серверов",
		"tui.plural.pack.one":    "%d пак",
		"tui.plural.pack.few":    "%d пака",
		"tui.plural.pack.many":   "%d паков",
		"tui.plural.addr.one":    "%d адрес",
		"tui.plural.addr.few":    "%d адреса",
		"tui.plural.addr.many":   "%d адресов",

		// Validation errors (encrypt pack-dir + address field).
		"tui.validate.enterPath":  "введите путь к папке пака",
		"tui.validate.notFolder":  "это не папка - укажите каталог ресурс-пака",
		"tui.validate.noManifest": "там нет manifest.json - укажите каталог ресурс-пака",
		"tui.validate.expectAddr": "ожидается host:port, например play.example.net:19132",

		// Recent-row status badge.
		"tui.recentStatus.ok":     "ок",
		"tui.recentStatus.failed": "ошибка",

		// Decrypt section.
		"tui.decrypt.badge.keys":      "ключи",
		"tui.decrypt.badge.decrypted": "расшифровано",
		"tui.decryptHelp.reDecrypt":   "Уже расшифровано - нажмите enter, чтобы расшифровать %s заново в папку decrypted/<server>/ рядом с паками.",
		"tui.decryptHelp.decrypt":     "Расшифровать %s - результат поместится в папку decrypted/<server>/ рядом с паками.",
		"tui.decryptHelp.packsGone":   "Ключи на месте, но паков нет - нажмите g, чтобы скачать и расшифровать их снова.",
		"tui.decryptHelp.noAddr":      "Ключи на месте, но паков нет, и нет сохранённого адреса для повторной загрузки.",

		// Featured filter line + empty states.
		"tui.featuredList.filter":    "фильтр: %s",
		"tui.featuredList.selHidden": "выбрано %d (%d скрыто фильтром)",
		"tui.featuredList.empty":     "Сейчас нет featured-серверов - попробуйте позже.",
		"tui.featuredList.noMatch":   "Нет серверов, подходящих под фильтр.",

		// Relative-time labels for the recent-downloads list (ageLabel).
		"tui.age.justNow":    "только что",
		"tui.age.minutesAgo": "%d мин назад",
		"tui.age.hoursAgo":   "%d ч назад",
		"tui.age.daysAgo":    "%d дн назад",

		// Hint-bar labels.
		"tui.hint.cancel":          "отмена",
		"tui.hint.move":            "перемещение",
		"tui.hint.open":            "открыть",
		"tui.hint.quit":            "выход",
		"tui.hint.back":            "назад",
		"tui.hint.select":          "выбрать",
		"tui.hint.continue":        "продолжить",
		"tui.hint.resolveIPs":      "определить IP",
		"tui.hint.filter":          "фильтр",
		"tui.hint.clearFilter":     "сбросить фильтр",
		"tui.hint.save":            "сохранить",
		"tui.hint.forget":          "забыть",
		"tui.hint.moveCaret":       "двигать курсор",
		"tui.hint.encrypt":         "зашифровать",
		"tui.hint.yes":             "да",
		"tui.hint.start":           "пуск",
		"tui.hint.pause":           "пауза",
		"tui.hint.resume":          "продолжить",
		"tui.hint.decrypt":         "расшифровать",
		"tui.hint.downloadDecrypt": "скачать+расшифровать",
		"tui.hint.backToMenu":      "в меню",
	})
}
