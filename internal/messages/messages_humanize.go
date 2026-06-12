package messages

import "github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

func init() {
	lang.Register(lang.English, map[string]string{
		// Mojang service discovery (substring fallback for mctoken.go wraps).
		"humanize.discovery.headline": "Mojang service discovery failed",
		"humanize.discovery.body":     "We couldn't fetch the service catalog from client.discovery.minecraft-services.net. The franchise chain can't start without it.",
		"humanize.discovery.fix":      "Almost always either a transient Mojang outage or your network blocking minecraft-services.net. Retry in a few minutes; if it keeps happening, confirm reachability with `curl -v https://client.discovery.minecraft-services.net/`.",

		// `connection to <server> failed:` protocol-mismatch branch.
		"humanize.protocol.headline": "%s is running a different protocol version",
		"humanize.protocol.body":     "The server's Minecraft Bedrock version doesn't match what this tool's gophertunnel was built against.",
		"humanize.protocol.fix":      "Wait for a tool update that targets the new Bedrock protocol, or downgrade the server temporarily.",

		// `connection to <server> failed:` login-handshake kick branch.
		"humanize.kick.headline": "%s kicked us during the login handshake",
		"humanize.kick.body":     "The server accepted the connection but rejected the session - often whitelist, ban, or anti-bot.",
		"humanize.kick.fix":      "If the same MSA can join in-game and not here, the server may be filtering on user-agent / client signature. There's no general fix from the tool side.",

		// `connection to <server> failed:` generic RakNet handshake branch.
		"humanize.raknet.headline": "Couldn't connect to %s",
		"humanize.raknet.body":     "The RakNet handshake failed. The inner error has the protocol-level details.",
		"humanize.raknet.fix":      "Confirm the host:port is reachable from the in-game Servers tab. If it works there but not here, capture both sessions with Wireshark and compare the first few packets.",

		// franchise.ErrExperienceOffline
		"humanize.venue.headline": "That slot has no active venue right now",
		"humanize.venue.body":     "Live Events and a few partner slots only resolve to a server during their event window. Outside the window Mojang returns 404 on the join endpoint.",
		"humanize.venue.fix": "Re-run `bedrock-pack-tools featured` later - the venue address will appear when the slot is live.\n" +
			"Or pick a different index from the list; entries that already show host:port are joinable anytime.",

		// franchise.ErrForbidden
		"humanize.forbidden.headline": "Mojang won't let this account reach that slot",
		"humanize.forbidden.body":     "Your token is valid (other entries resolve fine), but Mojang returned 403 Forbidden for this one. Some experiences and events are region-locked or only joinable from the official client.",
		"humanize.forbidden.fix": "Pick a different entry, ideally one that already shows host:port.\n" +
			"This isn't a token problem - re-authenticating won't change it.",

		// franchise.ErrAuthRejected
		"humanize.authrejected.headline": "Xbox identity was rejected by Mojang's franchise services",
		"humanize.authrejected.body":     "We re-minted the token once and Mojang still rejected it. That usually means the underlying Microsoft account itself is now in a bad state, not just our cache.",
		"humanize.authrejected.fix": "Delete the cached tokens and re-authenticate from scratch:\n" +
			"  rm \"%s\"\n" +
			"  rm \"%s\"\n" +
			"Then re-run. If it still fails, the MSA probably needs attention at account.microsoft.com.",

		// errPackNoManifest
		"humanize.nomanifest.headline": "That folder isn't a valid resource pack",
		"humanize.nomanifest.body":     "A Bedrock pack must have manifest.json at its top level with a header.uuid field. We couldn't find it.",
		"humanize.nomanifest.fix":      "Make sure you're pointing at the directory that contains manifest.json directly (not its parent). If you unzipped a .mcpack, the manifest is one level inside.",

		// errPackBadManifest
		"humanize.badmanifest.headline": "manifest.json is unreadable or malformed",
		"humanize.badmanifest.body":     "The file exists but we couldn't read it or parse it as JSON.",
		"humanize.badmanifest.fix":      "Open manifest.json and check it's valid JSON (trailing commas, smart quotes, and BOMs are common culprits).",

		// errPackBadKeyLen
		"humanize.badkeylen.headline": "Key length is wrong",
		"humanize.badkeylen.body":     "Bedrock pack keys are exactly 32 ASCII characters (raw, not hex-encoded, not base64).",
		"humanize.badkeylen.fix":      "Copy the key directly from keys.json - the whole string between the quotes, no surrounding whitespace.",

		// errPackWrongKey
		"humanize.wrongkey.headline": "Decryption failed - likely the wrong key for this pack",
		"humanize.wrongkey.body":     "Bedrock pack keys are pack-specific. Using a key from a different pack produces unreadable output.",
		"humanize.wrongkey.fix": "Open the keys.json you got from `download` (or the partner's keys file) and look up the key by the pack's UUID (header.uuid in manifest.json).\n" +
			"If you only have one keys.json and one pack, double-check the pack UUID matches a key entry.",

		// errPackTruncated
		"humanize.truncated.headline": "Pack file is corrupted (contents.json truncated)",
		"humanize.truncated.body":     "The pack's contents.json is shorter than the encryption header it should start with - the download was likely interrupted.",
		"humanize.truncated.fix":      "Re-run `bedrock-pack-tools download` against the same server, or grab the pack again from wherever it came from.",

		// errPackBadProtocol
		"humanize.badprotocol.headline": "Server sent an unexpected pack-info payload",
		"humanize.badprotocol.body":     "The ResourcePacksInfo packet didn't decode against the protocol shape gophertunnel was built against. Usually this means Mojang shipped a new Bedrock version and the tool hasn't caught up.",
		"humanize.badprotocol.fix":      "Check the latest release at github.com/iteplenky/bedrock-pack-tools and update. If you're already on latest, please open an issue with the server address and your Bedrock client version.",

		// errPackBadZip
		"humanize.badzip.headline": "Pack download was incomplete or not a valid zip",
		"humanize.badzip.body":     "We received pack bytes but couldn't open them as a zip archive. Most often a truncated transfer.",
		"humanize.badzip.fix":      "Re-run the download; transient transfer failures usually clear on the next attempt.",

		// errPackEmpty
		"humanize.empty.headline": "Pack has nothing to encrypt",
		"humanize.empty.body":     "Encryption produces a Bedrock-loadable .mcpack only when there's at least one resource file beyond manifest.json and pack_icon.png. We didn't find any.",
		"humanize.empty.fix": "Confirm you're pointing at the pack root and that it contains the textures / behaviour / sound files you expect.\n" +
			"If you really wanted to ship just the manifest, use any zip tool - encryption adds no value here.",

		// classifyOAuth: device-code timing (authorization_pending / expired_token / slow_down)
		"humanize.oauthtiming.headline": "Microsoft sign-in didn't complete in time",
		"humanize.oauthtiming.body":     "The Xbox Live device-code prompt expired before you finished entering the code at microsoft.com/link.",
		"humanize.oauthtiming.fix":      "Re-run the same command. The new code only lives ~15 minutes, so finish the browser step promptly.",

		// classifyOAuth: invalid_grant
		"humanize.oauthgrant.headline": "Cached Microsoft refresh token is no longer valid",
		"humanize.oauthgrant.body":     "Microsoft revoked or aged-out the refresh token in `%s`. That happens after long inactivity, password resets, or 2FA changes.",
		"humanize.oauthgrant.fix":      "Delete the cached token and re-authenticate from scratch:\n  rm \"%s\"",

		// classifyOAuth: generic fallback
		"humanize.oauthgeneric.headline": "Microsoft sign-in failed (OAuth `%s`)",
		"humanize.oauthgeneric.body":     "Microsoft's OAuth endpoint returned an error we don't have a specific message for.",
		"humanize.oauthgeneric.fix":      "Retry once. If it persists, delete the cached token at `%s` and authenticate from scratch.",

		// classifyFS: fs.ErrPermission
		"humanize.fsperm.headline": "Permission denied writing to disk",
		"humanize.fsperm.body":     "The OS refused the file or directory operation. The tool can't create the output it needs.",
		"humanize.fsperm.fix":      "Pass an output directory you own as the last argument (e.g. `~/some-folder`), or re-run with appropriate permissions on the existing target.",

		// classifyFS: ENOSPC
		"humanize.fsnospace.headline": "Disk is full",
		"humanize.fsnospace.body":     "The OS reported the target filesystem is out of space.",
		"humanize.fsnospace.fix":      "Free some space or point output at a different volume.",

		// classifyFS: EROFS
		"humanize.fsrofs.headline": "Target is on a read-only filesystem",
		"humanize.fsrofs.body":     "Common on macOS when writing under /System or /Volumes/Macintosh HD (the signed system volume).",
		"humanize.fsrofs.fix":      "Point output somewhere writable - your home directory is a safe default.",

		// classifyTLS: UnknownAuthorityError
		"humanize.tlsunknownca.headline": "TLS handshake failed for %s",
		"humanize.tlsunknownca.body":     "The server's certificate was signed by a CA your system doesn't trust. The most common cause is a corporate HTTPS-inspecting proxy.",
		"humanize.tlsunknownca.causes": "Corporate / school HTTPS-inspecting proxy (Zscaler, Netskope, Palo Alto)\n" +
			"Out-of-date system CA bundle (rare; older Linux distros)",
		"humanize.tlsunknownca.fix": "If you're on a corporate network, ask IT for the proxy's root CA and install it system-wide.\n" +
			"On a personal machine try the command off this network (mobile hotspot) to confirm.",

		// classifyTLS: CertificateInvalidError, Expired
		"humanize.tlsexpired.headline": "TLS certificate looks expired for %s",
		"humanize.tlsexpired.body":     "The server's cert is past its validity window from your machine's point of view. Most often that's a wrong system clock, not an actual cert problem.",
		"humanize.tlsexpired.fix":      "Check your system clock. On macOS: `sudo sntp -sS time.apple.com`. On Linux: `timedatectl status`.",

		// classifyTLS: CertificateInvalidError, other reasons
		"humanize.tlsinvalid.headline": "TLS certificate is invalid for %s",
		"humanize.tlsinvalid.body":     "The cert failed validation: %s.",
		"humanize.tlsinvalid.fix":      "Confirm the URL and system clock. If the issue persists, capture the cert with `openssl s_client -connect %s:443` for diagnostics.",

		// classifyTLS: stringified handshake/bad-certificate fallback
		"humanize.tlshandshake.headline": "TLS handshake failed for %s",
		"humanize.tlshandshake.body":     "The TLS layer rejected the connection before getting to the application protocol.",
		"humanize.tlshandshake.fix":      "Often a corporate HTTPS proxy or an outdated CA bundle. Try the command on a different network to isolate the cause.",

		// classifyNet: ENETUNREACH / EHOSTUNREACH
		"humanize.noroute.headline": "No route to %s",
		"humanize.noroute.body":     "Your machine has no route at all to reach that IP. Often a wifi or VPN that's only half-up.",
		"humanize.noroute.fix":      "Toggle wifi / VPN. If you're on a corporate VPN, confirm split-tunnel rules let Microsoft endpoints out.",

		// dnsDiag
		"humanize.dns.headline": "DNS lookup failed for %s",
		"humanize.dns.body":     "Your system couldn't resolve the hostname to an IP. The tool never got far enough to make a network request.",
		"humanize.dns.causes": "Captive portal (hotel/cafe wifi that hasn't been signed into yet)\n" +
			"DNS server outage or misconfigured resolver\n" +
			"VPN with broken split-DNS",
		"humanize.dns.fix": "Try `nslookup %s`.\n" +
			"If it also fails, switch DNS to 1.1.1.1 or 8.8.8.8 and retry.",

		// refusedDiag: game-server variant
		"humanize.refusedgame.headline": "%s refused the connection",
		"humanize.refusedgame.body":     "The host is reachable but nothing's listening on that port. Usually means the server is offline, the port is wrong, or you've been IP-banned.",
		"humanize.refusedgame.fix": "Confirm the address is right (default Bedrock port is 19132).\n" +
			"If you can join from the in-game Servers list but not via this tool, your IP may be banned at the network level.",

		// refusedDiag: upstream-service variant
		"humanize.refused.headline": "Connection refused by %s",
		"humanize.refused.body":     "The host is reachable but actively refused our connection. For Microsoft / Mojang services this is almost always a temporary outage on their side.",
		"humanize.refused.fix":      "Wait a few minutes and retry. If it persists for more than ~30 min, check https://xnotify.xboxlive.com/servicestatus.",

		// appLayerKickDiag
		"humanize.applayerkick.bodybase":   "RakNet handshake and Xbox sign-in both succeeded. The server's app layer rejected the session afterwards - typical anti-bot heuristic on big Bedrock networks.",
		"humanize.applayerkick.bodyreason": "\n\nReason returned by the server:\n  %s",
		"humanize.applayerkick.headline":   "%s kicked us after the handshake",
		"humanize.applayerkick.fix":        "Many large Bedrock partner servers run anti-bot heuristics that reject any client whose packet fingerprint doesn't match the official client. There's no general workaround from the tool side - those servers don't want third-party clients through.",

		// timeoutDiag: game-server variant
		"humanize.timeoutgame.headline": "Couldn't reach the Bedrock server at %s",
		"humanize.timeoutgame.body":     "Connection timed out at the RakNet layer. Either the server is offline, the address is wrong, or something between you and it is dropping UDP.",
		"humanize.timeoutgame.causes": "Server is offline or restarting\n" +
			"Wrong host / port (default is 19132)\n" +
			"ISP / firewall blocks outbound UDP",
		"humanize.timeoutgame.fix": "Try the same address in the in-game Servers tab.\n" +
			"If the game can connect but the tool can't, check whether anything filters UDP on your network.",

		// timeoutDiag: Microsoft-host variant
		"humanize.timeoutms.headline": "Couldn't reach %s",
		"humanize.timeoutms.body":     "%s isn't responding from your network.",
		"humanize.timeoutms.causes": "ISP-level blocking of Xbox / Microsoft services in your region\n" +
			"Corporate, school, or hotel firewall\n" +
			"Aggressive antivirus or \"Family Safety\" software blocking xboxlive.com\n" +
			"VPN required by your network (rare but seen)",
		"humanize.timeoutms.fix": "Quick check: `curl -v https://%s/`\n" +
			"If curl also hangs, it's your network - use a VPN to a region where Xbox Live works (most EU/US locations are fine).",

		// timeoutDiag: generic variant
		"humanize.timeout.headline": "Couldn't reach %s",
		"humanize.timeout.body":     "The request timed out without reaching the server.",
		"humanize.timeout.causes": "The server is offline or overloaded\n" +
			"Your network blocks the destination (firewall, ISP, VPN split-tunnel)",
		"humanize.timeout.fix": "Try again in a few minutes. If it keeps failing, capture the chain in Details below and confirm reachability with curl or your browser.",

		// classifyXSTS: account banned
		"humanize.xstsbanned.headline": "This Microsoft account is banned from Xbox Live",
		"humanize.xstsbanned.body":     "Microsoft's identity service rejected the sign-in with an account-ban code. We can't work around it from the tool.",
		"humanize.xstsbanned.fix":      "Use a different Microsoft account. Bans are a Microsoft policy decision, appealable only via account.microsoft.com.",

		// classifyXSTS: no Xbox profile
		"humanize.xstsnoprofile.headline": "This Microsoft account doesn't have an Xbox profile yet",
		"humanize.xstsnoprofile.body":     "Every MSA needs to be associated with an Xbox gamertag once before the franchise chain works.",
		"humanize.xstsnoprofile.fix": "Open xbox.com and sign in with this MSA to create the profile.\n" +
			"Once the gamertag prompt completes, re-run the same command - the tool will pick up the existing token.",

		// classifyXSTS: region not allowed
		"humanize.xstsregion.headline": "This account's region doesn't allow Xbox Live",
		"humanize.xstsregion.body":     "The Microsoft account's set country is one where Xbox Live isn't operated (or is sanctioned).",
		"humanize.xstsregion.fix": "Change the account region at account.microsoft.com/profile to one Xbox Live supports, then retry.\n" +
			"Or use a different MSA that's already in a supported region.",

		// classifyXSTS: parental consent
		"humanize.xstschild.headline": "Xbox Live needs parental consent for this account",
		"humanize.xstschild.body":     "The account is flagged as a child / family member and a parent hasn't approved Xbox Live access yet.",
		"humanize.xstschild.fix":      "Sign in to xbox.com as the family organiser and approve Xbox Live for this account, then retry.",

		// classifyXSTS: generic xbox-auth fallback
		"humanize.xstsgeneric.headline": "Microsoft sign-in failed during the Xbox handshake",
		"humanize.xstsgeneric.body":     "The MSA -> Xbox Live -> PlayFab chain rejected the credentials with a non-network error.",
		"humanize.xstsgeneric.fix":      "Re-running often clears transient hiccups. If it persists, delete the cached token at `%s` and authenticate from scratch.",

		// writeDiagnostic chrome
		"humanize.render.errorprefix":  "Error: ",
		"humanize.render.causeslabel":  "Common causes:",
		"humanize.render.detailslabel": "Details (paste into bug reports):",

		// Fallback host placeholders used inside diagnostics.
		"humanize.placeholder.upstream":     "the upstream service",
		"humanize.placeholder.upstreamhost": "the upstream host",
		"humanize.placeholder.host":         "<host>",
		"humanize.placeholder.dnshost":      "device.auth.xboxlive.com",

		// friendlyService names
		"humanize.service.mojang":   "Mojang's franchise services",
		"humanize.service.mssignin": "Microsoft sign-in",
	})

	lang.Register(lang.Russian, map[string]string{
		// Mojang service discovery
		"humanize.discovery.headline": "Не удалось получить каталог сервисов Mojang",
		"humanize.discovery.body":     "Не удалось загрузить каталог сервисов с client.discovery.minecraft-services.net. Без него цепочка franchise не запускается.",
		"humanize.discovery.fix":      "Почти всегда это либо временный сбой на стороне Mojang, либо ваша сеть блокирует minecraft-services.net. Повторите попытку через несколько минут; если повторяется - проверьте доступность командой `curl -v https://client.discovery.minecraft-services.net/`.",

		// protocol-mismatch branch
		"humanize.protocol.headline": "%s работает на другой версии протокола",
		"humanize.protocol.body":     "Версия Minecraft Bedrock на сервере не совпадает с той, под которую скомпилирован gophertunnel этого инструмента.",
		"humanize.protocol.fix":      "Дождитесь обновления инструмента под новый протокол Bedrock или временно понизьте версию сервера.",

		// login-handshake kick branch
		"humanize.kick.headline": "%s отключил нас во время входа",
		"humanize.kick.body":     "Сервер принял подключение, но отклонил сессию - часто это whitelist, бан или анти-бот.",
		"humanize.kick.fix":      "Если тот же аккаунт MSA заходит в игре, но не здесь, сервер, возможно, фильтрует по user-agent / сигнатуре клиента. Общего решения со стороны инструмента нет.",

		// generic RakNet handshake branch
		"humanize.raknet.headline": "Не удалось подключиться к %s",
		"humanize.raknet.body":     "Не удался handshake RakNet. Подробности уровня протокола - во вложенной ошибке.",
		"humanize.raknet.fix":      "Убедитесь, что host:port доступен из вкладки серверов в игре. Если там работает, а здесь нет, перехватите обе сессии Wireshark и сравните первые пакеты.",

		// franchise.ErrExperienceOffline
		"humanize.venue.headline": "У этого слота сейчас нет активной площадки",
		"humanize.venue.body":     "Live Events и часть партнёрских слотов доступны только в окно своего события. Вне окна Mojang возвращает 404 на endpoint входа.",
		"humanize.venue.fix": "Запустите `bedrock-pack-tools featured` позже - адрес площадки появится, когда слот станет активным.\n" +
			"Или выберите другой индекс из списка; записи, где уже показан host:port, доступны в любое время.",

		// franchise.ErrForbidden
		"humanize.forbidden.headline": "Mojang не пускает этот аккаунт к этому слоту",
		"humanize.forbidden.body":     "Ваш токен действителен (другие записи открываются нормально), но для этой записи Mojang вернул 403 Forbidden. Часть experiences и событий привязаны к региону или доступны только из официального клиента.",
		"humanize.forbidden.fix": "Выберите другую запись, лучше ту, где уже показан host:port.\n" +
			"Это не проблема токена - повторная авторизация ничего не изменит.",

		// franchise.ErrAuthRejected
		"humanize.authrejected.headline": "Сервисы franchise Mojang отклонили идентификацию Xbox",
		"humanize.authrejected.body":     "Мы перевыпустили токен один раз, и Mojang всё равно его отклонил. Обычно это значит, что сам аккаунт Microsoft теперь в плохом состоянии, а не только наш кэш.",
		"humanize.authrejected.fix": "Удалите кэшированные токены и авторизуйтесь заново с нуля:\n" +
			"  rm \"%s\"\n" +
			"  rm \"%s\"\n" +
			"Затем повторите команду. Если по-прежнему не работает, аккаунту MSA, вероятно, нужно внимание на account.microsoft.com.",

		// errPackNoManifest
		"humanize.nomanifest.headline": "Эта папка не является корректным ресурс-паком",
		"humanize.nomanifest.body":     "В паке Bedrock на верхнем уровне должен быть manifest.json с полем header.uuid. Мы его не нашли.",
		"humanize.nomanifest.fix":      "Убедитесь, что указываете на папку, которая содержит manifest.json напрямую (а не на её родителя). Если вы распаковали .mcpack, манифест находится на один уровень внутри.",

		// errPackBadManifest
		"humanize.badmanifest.headline": "manifest.json не читается или повреждён",
		"humanize.badmanifest.body":     "Файл существует, но мы не смогли прочитать его или распарсить как JSON.",
		"humanize.badmanifest.fix":      "Откройте manifest.json и проверьте, что это корректный JSON (висячие запятые, «умные» кавычки и BOM - частые причины).",

		// errPackBadKeyLen
		"humanize.badkeylen.headline": "Неверная длина ключа",
		"humanize.badkeylen.body":     "Ключи паков Bedrock - ровно 32 ASCII-символа (в сыром виде, не hex, не base64).",
		"humanize.badkeylen.fix":      "Скопируйте ключ напрямую из keys.json - всю строку между кавычками, без пробелов по краям.",

		// errPackWrongKey
		"humanize.wrongkey.headline": "Расшифровка не удалась - вероятно, неверный ключ для этого пака",
		"humanize.wrongkey.body":     "Ключи паков Bedrock индивидуальны для каждого пака. Ключ от другого пака даёт нечитаемый результат.",
		"humanize.wrongkey.fix": "Откройте keys.json, полученный из `download` (или файл ключей партнёра), и найдите ключ по UUID пака (header.uuid в manifest.json).\n" +
			"Если у вас один keys.json и один пак, перепроверьте, что UUID пака совпадает с записью ключа.",

		// errPackTruncated
		"humanize.truncated.headline": "Файл пака повреждён (contents.json обрезан)",
		"humanize.truncated.body":     "Файл contents.json пака короче, чем заголовок шифрования, с которого он должен начинаться - загрузка, скорее всего, прервалась.",
		"humanize.truncated.fix":      "Запустите `bedrock-pack-tools download` повторно для того же сервера или получите пак заново оттуда, откуда он пришёл.",

		// errPackBadProtocol
		"humanize.badprotocol.headline": "Сервер прислал неожиданный pack-info payload",
		"humanize.badprotocol.body":     "Пакет ResourcePacksInfo не разобрался по схеме протокола, под которую скомпилирован gophertunnel. Обычно это значит, что Mojang выпустил новую версию Bedrock, а инструмент ещё не догнал.",
		"humanize.badprotocol.fix":      "Проверьте последний релиз на github.com/iteplenky/bedrock-pack-tools и обновитесь. Если у вас уже последняя версия, откройте issue с адресом сервера и версией вашего клиента Bedrock.",

		// errPackBadZip
		"humanize.badzip.headline": "Загрузка пака не завершилась или это не корректный zip",
		"humanize.badzip.body":     "Мы получили байты пака, но не смогли открыть их как zip-архив. Чаще всего это обрезанная передача.",
		"humanize.badzip.fix":      "Запустите загрузку повторно; временные сбои передачи обычно проходят со следующей попытки.",

		// errPackEmpty
		"humanize.empty.headline": "В паке нечего шифровать",
		"humanize.empty.body":     "Шифрование создаёт загружаемый в Bedrock .mcpack только при наличии хотя бы одного ресурсного файла помимо manifest.json и pack_icon.png. Мы таких не нашли.",
		"humanize.empty.fix": "Убедитесь, что указываете на корень пака и что в нём есть ожидаемые файлы текстур / поведения / звуков.\n" +
			"Если вы действительно хотели упаковать только манифест, используйте любой zip-инструмент - шифрование здесь ничего не даёт.",

		// classifyOAuth: device-code timing
		"humanize.oauthtiming.headline": "Вход Microsoft не завершился вовремя",
		"humanize.oauthtiming.body":     "Код устройства Xbox Live истёк, прежде чем вы успели ввести его на microsoft.com/link.",
		"humanize.oauthtiming.fix":      "Запустите ту же команду снова. Новый код живёт всего ~15 минут, так что завершайте шаг в браузере быстро.",

		// classifyOAuth: invalid_grant
		"humanize.oauthgrant.headline": "Кэшированный refresh-токен Microsoft больше недействителен",
		"humanize.oauthgrant.body":     "Microsoft отозвал или просрочил refresh-токен в `%s`. Так бывает после долгого простоя, сброса пароля или изменений 2FA.",
		"humanize.oauthgrant.fix":      "Удалите кэшированный токен и авторизуйтесь заново с нуля:\n  rm \"%s\"",

		// classifyOAuth: generic fallback
		"humanize.oauthgeneric.headline": "Вход Microsoft не удался (OAuth `%s`)",
		"humanize.oauthgeneric.body":     "Endpoint OAuth Microsoft вернул ошибку, для которой у нас нет отдельного сообщения.",
		"humanize.oauthgeneric.fix":      "Повторите один раз. Если повторяется, удалите кэшированный токен по пути `%s` и авторизуйтесь с нуля.",

		// classifyFS: fs.ErrPermission
		"humanize.fsperm.headline": "Нет прав на запись на диск",
		"humanize.fsperm.body":     "ОС отклонила операцию с файлом или папкой. Инструмент не может создать нужный ему результат.",
		"humanize.fsperm.fix":      "Укажите последним аргументом папку для вывода, которой вы владеете (например, `~/some-folder`), или запустите с подходящими правами на существующую цель.",

		// classifyFS: ENOSPC
		"humanize.fsnospace.headline": "Диск заполнен",
		"humanize.fsnospace.body":     "ОС сообщила, что на целевой файловой системе закончилось место.",
		"humanize.fsnospace.fix":      "Освободите место или укажите вывод на другой том.",

		// classifyFS: EROFS
		"humanize.fsrofs.headline": "Цель находится на файловой системе только для чтения",
		"humanize.fsrofs.body":     "Часто на macOS при записи в /System или /Volumes/Macintosh HD (подписанный системный том).",
		"humanize.fsrofs.fix":      "Укажите вывод туда, куда можно писать; домашняя папка - безопасный вариант по умолчанию.",

		// classifyTLS: UnknownAuthorityError
		"humanize.tlsunknownca.headline": "Не прошёл TLS handshake для %s",
		"humanize.tlsunknownca.body":     "Сертификат сервера подписан CA, которому ваша система не доверяет. Чаще всего причина - корпоративный прокси, инспектирующий HTTPS.",
		"humanize.tlsunknownca.causes": "Корпоративный / школьный прокси, инспектирующий HTTPS (Zscaler, Netskope, Palo Alto)\n" +
			"Устаревший системный набор CA (редко; старые дистрибутивы Linux)",
		"humanize.tlsunknownca.fix": "Если вы в корпоративной сети, запросите у IT корневой CA прокси и установите его в систему.\n" +
			"На личной машине попробуйте команду вне этой сети (мобильная точка доступа), чтобы убедиться.",

		// classifyTLS: CertificateInvalidError, Expired
		"humanize.tlsexpired.headline": "Сертификат TLS выглядит просроченным для %s",
		"humanize.tlsexpired.body":     "Сертификат сервера просрочен с точки зрения вашей машины. Чаще всего дело в неверных системных часах, а не в реальной проблеме с сертификатом.",
		"humanize.tlsexpired.fix":      "Проверьте системные часы. На macOS: `sudo sntp -sS time.apple.com`. На Linux: `timedatectl status`.",

		// classifyTLS: CertificateInvalidError, other reasons
		"humanize.tlsinvalid.headline": "Сертификат TLS недействителен для %s",
		"humanize.tlsinvalid.body":     "Сертификат не прошёл проверку: %s.",
		"humanize.tlsinvalid.fix":      "Проверьте URL и системные часы. Если проблема сохраняется, перехватите сертификат командой `openssl s_client -connect %s:443` для диагностики.",

		// classifyTLS: stringified handshake/bad-certificate fallback
		"humanize.tlshandshake.headline": "Не прошёл TLS handshake для %s",
		"humanize.tlshandshake.body":     "Слой TLS отклонил соединение, не дойдя до прикладного протокола.",
		"humanize.tlshandshake.fix":      "Часто это корпоративный HTTPS-прокси или устаревший набор CA. Попробуйте команду в другой сети, чтобы локализовать причину.",

		// classifyNet: ENETUNREACH / EHOSTUNREACH
		"humanize.noroute.headline": "Нет маршрута до %s",
		"humanize.noroute.body":     "У вас нет маршрута до этого IP. Часто это wifi или VPN, поднятые лишь наполовину.",
		"humanize.noroute.fix":      "Переключите wifi / VPN. Если вы в корпоративном VPN, проверьте, что правила split-tunnel пропускают endpoint-ы Microsoft наружу.",

		// dnsDiag
		"humanize.dns.headline": "Не удалось разрешить DNS для %s",
		"humanize.dns.body":     "Ваша система не смогла разрешить имя хоста. Инструмент даже не дошёл до сетевого запроса.",
		"humanize.dns.causes": "Кэптив-портал (wifi в отеле/кафе, в который ещё не вошли)\n" +
			"Сбой DNS-сервера или неправильно настроенный resolver\n" +
			"VPN с поломанным split-DNS",
		"humanize.dns.fix": "Попробуйте `nslookup %s`.\n" +
			"Если тоже не работает, смените DNS на 1.1.1.1 или 8.8.8.8 и повторите.",

		// refusedDiag: game-server variant
		"humanize.refusedgame.headline": "%s отклонил соединение",
		"humanize.refusedgame.body":     "Хост доступен, но на этом порту никто не слушает. Обычно это значит, что сервер выключен, порт неверный или вас забанили по IP.",
		"humanize.refusedgame.fix": "Проверьте, что адрес верный (порт Bedrock по умолчанию - 19132).\n" +
			"Если вы можете зайти из списка серверов в игре, но не через этот инструмент, ваш IP, возможно, забанен на сетевом уровне.",

		// refusedDiag: upstream-service variant
		"humanize.refused.headline": "Соединение отклонено хостом %s",
		"humanize.refused.body":     "Хост доступен, но активно отклонил наше соединение. Для сервисов Microsoft / Mojang это почти всегда временный сбой на их стороне.",
		"humanize.refused.fix":      "Подождите несколько минут и повторите. Если длится больше ~30 мин, проверьте https://xnotify.xboxlive.com/servicestatus.",

		// appLayerKickDiag
		"humanize.applayerkick.bodybase":   "Handshake RakNet и вход Xbox прошли успешно. Прикладной слой сервера отклонил сессию уже после - типичная анти-бот эвристика крупных сетей Bedrock.",
		"humanize.applayerkick.bodyreason": "\n\nПричина, возвращённая сервером:\n  %s",
		"humanize.applayerkick.headline":   "%s отключил нас после handshake",
		"humanize.applayerkick.fix":        "Многие крупные партнёрские серверы Bedrock используют анти-бот эвристики, которые отклоняют любого клиента, чей отпечаток пакетов не совпадает с официальным клиентом. Общего обходного пути со стороны инструмента нет - такие серверы не хотят пускать сторонние клиенты.",

		// timeoutDiag: game-server variant
		"humanize.timeoutgame.headline": "Не удалось подключиться к серверу Bedrock %s",
		"humanize.timeoutgame.body":     "Соединение истекло на уровне RakNet. Либо сервер выключен, либо адрес неверный, либо что-то между вами и им роняет UDP.",
		"humanize.timeoutgame.causes": "Сервер выключен или перезапускается\n" +
			"Неверный host / port (по умолчанию - 19132)\n" +
			"ISP / firewall блокирует исходящий UDP",
		"humanize.timeoutgame.fix": "Попробуйте тот же адрес во вкладке серверов в игре.\n" +
			"Если игра подключается, а инструмент нет, проверьте, не фильтрует ли что-то UDP в вашей сети.",

		// timeoutDiag: Microsoft-host variant
		"humanize.timeoutms.headline": "Не удалось достучаться до %s",
		"humanize.timeoutms.body":     "%s не отвечает из вашей сети.",
		"humanize.timeoutms.causes": "Блокировка сервисов Xbox / Microsoft на уровне ISP в вашем регионе\n" +
			"Корпоративный, школьный или гостиничный firewall\n" +
			"Антивирус или фильтр parental-controls, блокирующие xboxlive.com\n" +
			"VPN, требуемый вашей сетью (редко, но встречается)",
		"humanize.timeoutms.fix": "Быстрая проверка: `curl -v https://%s/`\n" +
			"Если curl тоже зависает, дело в вашей сети - используйте VPN в регион, где Xbox Live работает (большинство локаций EU/US подходят).",

		// timeoutDiag: generic variant
		"humanize.timeout.headline": "Не удалось достучаться до %s",
		"humanize.timeout.body":     "Запрос истёк, не дойдя до сервера.",
		"humanize.timeout.causes": "Сервер выключен или перегружен\n" +
			"Ваша сеть блокирует назначение (firewall, ISP, VPN split-tunnel)",
		"humanize.timeout.fix": "Попробуйте снова через несколько минут. Если продолжает падать, сохраните цепочку из раздела Details ниже и проверьте доступность через curl или браузер.",

		// classifyXSTS: account banned
		"humanize.xstsbanned.headline": "Этот аккаунт Microsoft заблокирован в Xbox Live",
		"humanize.xstsbanned.body":     "Microsoft вернул ошибку блокировки аккаунта при входе. Обойти это из инструмента нельзя.",
		"humanize.xstsbanned.fix":      "Используйте другой аккаунт Microsoft. Баны - это решение политики Microsoft, оспаривается только через account.microsoft.com.",

		// classifyXSTS: no Xbox profile
		"humanize.xstsnoprofile.headline": "У этого аккаунта Microsoft ещё нет профиля Xbox",
		"humanize.xstsnoprofile.body":     "Каждый аккаунт MSA нужно один раз связать с геймертегом Xbox, прежде чем цепочка franchise заработает.",
		"humanize.xstsnoprofile.fix": "Откройте xbox.com и войдите с этим аккаунтом MSA, чтобы создать профиль.\n" +
			"Когда запрос геймертега завершится, запустите ту же команду снова - инструмент подхватит существующий токен.",

		// classifyXSTS: region not allowed
		"humanize.xstsregion.headline": "Xbox Live недоступен в регионе этого аккаунта",
		"humanize.xstsregion.body":     "В аккаунте Microsoft указана страна, где Xbox Live не работает (или находится под санкциями).",
		"humanize.xstsregion.fix": "Смените регион аккаунта на account.microsoft.com/profile на тот, который поддерживает Xbox Live, и повторите.\n" +
			"Или используйте другой аккаунт MSA, уже находящийся в поддерживаемом регионе.",

		// classifyXSTS: parental consent
		"humanize.xstschild.headline": "Для этого аккаунта Xbox Live требует родительского согласия",
		"humanize.xstschild.body":     "Аккаунт помечен как детский / член семьи, и родитель ещё не одобрил доступ к Xbox Live.",
		"humanize.xstschild.fix":      "Войдите на xbox.com как организатор семьи и одобрите Xbox Live для этого аккаунта, затем повторите.",

		// classifyXSTS: generic xbox-auth fallback
		"humanize.xstsgeneric.headline": "Вход Microsoft не удался во время handshake Xbox",
		"humanize.xstsgeneric.body":     "При входе через MSA -> Xbox Live -> PlayFab учётные данные были отклонены с ошибкой, не связанной с сетью.",
		"humanize.xstsgeneric.fix":      "Повторный запуск часто устраняет временные сбои. Если повторяется, удалите кэшированный токен по пути `%s` и авторизуйтесь с нуля.",

		// writeDiagnostic chrome
		"humanize.render.errorprefix":  "Ошибка: ",
		"humanize.render.causeslabel":  "Частые причины:",
		"humanize.render.detailslabel": "Details (вставьте в отчёт об ошибке):",

		// Fallback host placeholders
		"humanize.placeholder.upstream":     "вышестоящего сервиса",
		"humanize.placeholder.upstreamhost": "вышестоящего хоста",
		"humanize.placeholder.host":         "<host>",
		"humanize.placeholder.dnshost":      "device.auth.xboxlive.com",

		// friendlyService names
		"humanize.service.mojang":   "Mojang franchise-сервисы",
		"humanize.service.mssignin": "вход Microsoft",
	})
}
