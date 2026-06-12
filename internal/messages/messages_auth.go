package messages

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

func init() {
	lang.Register(lang.English, map[string]string{
		"auth.login.done":  "  Signed in.",
		"auth.logout.done": "  Signed out - cached tokens removed.",

		"auth.cached":      "  Auth: using cached Xbox token",
		"auth.start":       "  Auth: no cached token - starting Xbox Live device auth",
		"auth.prompt.hint": "  A URL and code will appear - enter it in your browser.",
		"auth.saved":       "  Auth: token saved",

		"auth.warn.token.resolve": "Warning: could not resolve token cache path: %v\n",
		"auth.warn.token.marshal": "Warning: could not marshal token: %v\n",
		"auth.warn.token.save":    "Warning: could not save token: %v\n",

		"auth.warn.mctoken.resolve": "Warning: could not resolve mctoken cache path: %v\n",
		"auth.warn.mctoken.marshal": "Warning: could not marshal mctoken: %v\n",
		"auth.warn.mctoken.save":    "Warning: could not save mctoken cache: %v\n",

		"auth.warn.device.notPersisted":  "Warning: device.ID not persisted (%v); cohort assignment will be unstable\n",
		"auth.warn.device.persistFailed": "Warning: could not persist device.ID: %v\n",
	})
	lang.Register(lang.Russian, map[string]string{
		"auth.login.done":  "  Вход выполнен.",
		"auth.logout.done": "  Выход выполнен - кэшированные токены удалены.",

		"auth.cached":      "  Auth: используется кэшированный токен Xbox",
		"auth.start":       "  Auth: кэшированного токена нет - начинаем вход в Xbox Live по коду устройства",
		"auth.prompt.hint": "  Появятся URL и код - введите его в браузер.",
		"auth.saved":       "  Auth: токен сохранён",

		"auth.warn.token.resolve": "Внимание: не удалось определить путь к кэшу токена: %v\n",
		"auth.warn.token.marshal": "Внимание: не удалось сериализовать токен: %v\n",
		"auth.warn.token.save":    "Внимание: не удалось сохранить токен: %v\n",

		"auth.warn.mctoken.resolve": "Внимание: не удалось определить путь к кэшу mctoken: %v\n",
		"auth.warn.mctoken.marshal": "Внимание: не удалось сериализовать mctoken: %v\n",
		"auth.warn.mctoken.save":    "Внимание: не удалось сохранить кэш mctoken: %v\n",

		"auth.warn.device.notPersisted":  "Внимание: device.ID не сохранён (%v); когорта будет нестабильной\n",
		"auth.warn.device.persistFailed": "Внимание: не удалось сохранить device.ID: %v\n",
	})
}
