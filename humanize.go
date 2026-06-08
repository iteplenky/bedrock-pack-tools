package main

import (
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"regexp"
	"strings"
	"syscall"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"golang.org/x/oauth2"
)

// diagnostic augments the raw error chain with user-friendly text.
// The raw chain is still printed under "Details:" for bug reports.
type diagnostic struct {
	headline string
	body     string
	causes   []string
	fix      string
}

// Sentinels for fmt.Errorf wraps that humanize classifies via errors.Is.
var (
	errPackNoManifest  = errors.New("pack: missing or invalid manifest.json")
	errPackBadManifest = errors.New("pack: manifest.json unreadable")
	errPackBadKeyLen   = errors.New("pack: key length must be 32")
	errPackWrongKey    = errors.New("pack: decryption failed (likely wrong key)")
	errPackTruncated   = errors.New("pack: contents.json truncated")
	errPackBadProtocol = errors.New("pack: malformed ResourcePacksInfo")
	errPackBadZip      = errors.New("pack: download incomplete or not a zip")
	errPackEmpty       = errors.New("pack: no encryptable content")
)

// humanize returns a diagnostic for err, or (_, false) when no
// pattern matches. Typed checks (errors.Is/As) win over substring;
// substring is only used where the upstream lib hides info in the
// message string (notably XSTS codes from gophertunnel/minecraft/auth).
func humanize(err error) (diagnostic, bool) {
	if err == nil {
		return diagnostic{}, false
	}

	if d, ok := classifyOurSentinels(err); ok {
		return d, true
	}
	if d, ok := classifyOAuth(err); ok {
		return d, true
	}
	if d, ok := classifyFS(err); ok {
		return d, true
	}
	if d, ok := classifyTLS(err); ok {
		return d, true
	}
	if d, ok := classifyNet(err); ok {
		return d, true
	}

	msg := err.Error()
	low := strings.ToLower(msg)

	if d, ok := classifyXSTS(msg, low); ok {
		return d, true
	}

	// Substring fallback for the mctoken.go discovery wraps.
	if strings.Contains(msg, "discover services:") || strings.Contains(msg, "decode auth environment") {
		return diagnostic{
			headline: "Mojang service discovery failed",
			body:     "We couldn't fetch the service catalog from client.discovery.minecraft-services.net. The franchise chain can't start without it.",
			fix:      "Almost always either a transient Mojang outage or your network blocking minecraft-services.net. Retry in a few minutes; if it keeps happening, confirm reachability with `curl -v https://client.discovery.minecraft-services.net/`.",
		}, true
	}

	// `connection to <server> failed:` wraps live in keys.go / download.go.
	if game := firstMatch(reGameServer, msg); game != "" {
		switch {
		case strings.Contains(msg, "dial minecraft "):
			// `dial minecraft <addr>->...` is the gophertunnel marker for
			// "RakNet + Xbox handshake both succeeded; server kicked us
			// after". Check this BEFORE the protocol-keyword branch -
			// some servers send custom kick messages mentioning
			// "outdated" or "protocol" as anti-bot rhetoric, not as a
			// real version mismatch. The presence of dial minecraft
			// proves handshake finished, so it's an app-layer kick
			// regardless of what the kick text says.
			return appLayerKickDiag(game, msg), true
		case containsAny(low, "protocol", "incompatible", "outdated"):
			return diagnostic{
				headline: game + " is running a different protocol version",
				body:     "The server's Minecraft Bedrock version doesn't match what this tool's gophertunnel was built against.",
				fix:      "Wait for a tool update that targets the new Bedrock protocol, or downgrade the server temporarily.",
			}, true
		case containsAny(low, "kicked", "disconnect", "banned"):
			return diagnostic{
				headline: game + " kicked us during the login handshake",
				body:     "The server accepted the connection but rejected the session - often whitelist, ban, or anti-bot.",
				fix:      "If the same MSA can join in-game and not here, the server may be filtering on user-agent / client signature. There's no general fix from the tool side.",
			}, true
		}
		return diagnostic{
			headline: "Couldn't connect to " + game,
			body:     "The RakNet handshake failed. The inner error has the protocol-level details.",
			fix:      "Confirm the host:port is reachable from the in-game Servers tab. If it works there but not here, capture both sessions with Wireshark and compare the first few packets.",
		}, true
	}

	return diagnostic{}, false
}

// classifyOurSentinels covers all our sentinels (errPack* here plus
// franchise.ErrAuthRejected and franchise.ErrExperienceOffline).
func classifyOurSentinels(err error) (diagnostic, bool) {
	switch {
	case errors.Is(err, franchise.ErrExperienceOffline):
		return diagnostic{
			headline: "That slot has no active venue right now",
			body:     "Live Events and a few partner slots only resolve to a server during their event window. Outside the window Mojang returns 404 on the join endpoint.",
			fix: "Re-run `bedrock-pack-tools featured` later - the venue address will appear when the slot is live.\n" +
				"Or pick a different index from the list; entries that already show host:port are joinable anytime.",
		}, true

	case errors.Is(err, franchise.ErrAuthRejected):
		// Reaching the user means the featured.go re-mint retry also failed.
		return diagnostic{
			headline: "Xbox identity was rejected by Mojang's franchise services",
			body:     "We re-minted the token once and Mojang still rejected it. That usually means the underlying Microsoft account itself is now in a bad state, not just our cache.",
			fix: "Delete the cached tokens and re-authenticate from scratch:\n" +
				"  rm \"" + tokenPath() + "\"\n" +
				"  rm \"" + mustMCTokenPath() + "\"\n" +
				"Then re-run. If it still fails, the MSA probably needs attention at account.microsoft.com.",
		}, true

	case errors.Is(err, errPackNoManifest):
		return diagnostic{
			headline: "That folder isn't a valid resource pack",
			body:     "A Bedrock pack must have manifest.json at its top level with a header.uuid field. We couldn't find it.",
			fix:      "Make sure you're pointing at the directory that contains manifest.json directly (not its parent). If you unzipped a .mcpack, the manifest is one level inside.",
		}, true

	case errors.Is(err, errPackBadManifest):
		return diagnostic{
			headline: "manifest.json is unreadable or malformed",
			body:     "The file exists but we couldn't read it or parse it as JSON.",
			fix:      "Open manifest.json and check it's valid JSON (trailing commas, smart quotes, and BOMs are common culprits).",
		}, true

	case errors.Is(err, errPackBadKeyLen):
		return diagnostic{
			headline: "Key length is wrong",
			body:     "Bedrock pack keys are exactly 32 ASCII characters (raw, not hex-encoded, not base64).",
			fix:      "Copy the key directly from keys.json - the whole string between the quotes, no surrounding whitespace.",
		}, true

	case errors.Is(err, errPackWrongKey):
		return diagnostic{
			headline: "Decryption failed - likely the wrong key for this pack",
			body:     "Bedrock pack keys are pack-specific. Using a key from a different pack produces unreadable output.",
			fix: "Open the keys.json you got from `download` (or the partner's keys file) and look up the key by the pack's UUID (header.uuid in manifest.json).\n" +
				"If you only have one keys.json and one pack, double-check the pack UUID matches a key entry.",
		}, true

	case errors.Is(err, errPackTruncated):
		return diagnostic{
			headline: "Pack file is corrupted (contents.json truncated)",
			body:     "The pack's contents.json is shorter than the encryption header it should start with - the download was likely interrupted.",
			fix:      "Re-run `bedrock-pack-tools download` against the same server, or grab the pack again from wherever it came from.",
		}, true

	case errors.Is(err, errPackBadProtocol):
		return diagnostic{
			headline: "Server sent an unexpected pack-info payload",
			body:     "The ResourcePacksInfo packet didn't decode against the protocol shape gophertunnel was built against. Usually this means Mojang shipped a new Bedrock version and the tool hasn't caught up.",
			fix:      "Check the latest release at github.com/iteplenky/bedrock-pack-tools and update. If you're already on latest, please open an issue with the server address and your Bedrock client version.",
		}, true

	case errors.Is(err, errPackBadZip):
		return diagnostic{
			headline: "Pack download was incomplete or not a valid zip",
			body:     "We received pack bytes but couldn't open them as a zip archive. Most often a truncated transfer.",
			fix:      "Re-run the download; transient transfer failures usually clear on the next attempt.",
		}, true

	case errors.Is(err, errPackEmpty):
		return diagnostic{
			headline: "Pack has nothing to encrypt",
			body:     "Encryption produces a Bedrock-loadable .mcpack only when there's at least one resource file beyond manifest.json and pack_icon.png. We didn't find any.",
			fix: "Confirm you're pointing at the pack root and that it contains the textures / behaviour / sound files you expect.\n" +
				"If you really wanted to ship just the manifest, use any zip tool - encryption adds no value here.",
		}, true
	}
	return diagnostic{}, false
}

// classifyOAuth picks off device-code timing errors from oauth2. The
// RetrieveError type carries the OAuth error code in a structured field,
// so we don't need to parse the response body.
func classifyOAuth(err error) (diagnostic, bool) {
	var rErr *oauth2.RetrieveError
	if !errors.As(err, &rErr) {
		return diagnostic{}, false
	}
	switch rErr.ErrorCode {
	case "authorization_pending", "expired_token", "slow_down":
		return diagnostic{
			headline: "Microsoft sign-in didn't complete in time",
			body:     "The Xbox Live device-code prompt expired before you finished entering the code at microsoft.com/link.",
			fix:      "Re-run the same command. The new code only lives ~15 minutes, so finish the browser step promptly.",
		}, true
	case "invalid_grant":
		return diagnostic{
			headline: "Cached Microsoft refresh token is no longer valid",
			body:     "Microsoft revoked or aged-out the refresh token in `" + tokenPath() + "`. That happens after long inactivity, password resets, or 2FA changes.",
			fix:      "Delete the cached token and re-authenticate from scratch:\n  rm \"" + tokenPath() + "\"",
		}, true
	}
	return diagnostic{
		headline: "Microsoft sign-in failed (OAuth `" + rErr.ErrorCode + "`)",
		body:     "Microsoft's OAuth endpoint returned an error we don't have a specific message for.",
		fix:      "Retry once. If it persists, delete the cached token at `" + tokenPath() + "` and authenticate from scratch.",
	}, true
}

// classifyFS catches permission / no-space / read-only. fs.ErrPermission
// matches EACCES/EPERM cross-platform without hard-coding errno.
func classifyFS(err error) (diagnostic, bool) {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return diagnostic{
			headline: "Permission denied writing to disk",
			body:     "The OS refused the file or directory operation. The tool can't create the output it needs.",
			fix:      "Pass an output directory you own as the last argument (e.g. `~/some-folder`), or re-run with appropriate permissions on the existing target.",
		}, true
	case errors.Is(err, syscall.ENOSPC):
		return diagnostic{
			headline: "Disk is full",
			body:     "The OS reported the target filesystem is out of space.",
			fix:      "Free some space or point output at a different volume.",
		}, true
	case errors.Is(err, syscall.EROFS):
		return diagnostic{
			headline: "Target is on a read-only filesystem",
			body:     "Common on macOS when writing under /System or /Volumes/Macintosh HD (the signed system volume).",
			fix:      "Point output somewhere writable - your home directory is a safe default.",
		}, true
	}
	return diagnostic{}, false
}

// classifyTLS uses x509's typed errors. UnknownAuthorityError fires on
// corporate HTTPS interception; CertificateInvalidError covers expiry.
func classifyTLS(err error) (diagnostic, bool) {
	host := extractHost(err)
	var ua x509.UnknownAuthorityError
	if errors.As(err, &ua) {
		return diagnostic{
			headline: "TLS handshake failed for " + nonEmpty(host, "the upstream service"),
			body:     "The server's certificate was signed by a CA your system doesn't trust. The most common cause is a corporate HTTPS-inspecting proxy.",
			causes: []string{
				"Corporate / school HTTPS-inspecting proxy (Zscaler, Netskope, Palo Alto)",
				"Out-of-date system CA bundle (rare; older Linux distros)",
			},
			fix: "If you're on a corporate network, ask IT for the proxy's root CA and install it system-wide.\n" +
				"On a personal machine try the command off this network (mobile hotspot) to confirm.",
		}, true
	}
	var ci x509.CertificateInvalidError
	if errors.As(err, &ci) {
		if ci.Reason == x509.Expired {
			return diagnostic{
				headline: "TLS certificate looks expired for " + nonEmpty(host, "the upstream service"),
				body:     "The server's cert is past its validity window from your machine's point of view. Most often that's a wrong system clock, not an actual cert problem.",
				fix:      "Check your system clock. On macOS: `sudo sntp -sS time.apple.com`. On Linux: `timedatectl status`.",
			}, true
		}
		return diagnostic{
			headline: "TLS certificate is invalid for " + nonEmpty(host, "the upstream service"),
			body:     "The cert failed validation: " + ci.Error() + ".",
			fix:      "Confirm the URL and system clock. If the issue persists, capture the cert with `openssl s_client -connect " + nonEmpty(host, "<host>") + ":443` for diagnostics.",
		}, true
	}
	// Substring fallback for stringified TLS errors.
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "tls: handshake failure") || strings.Contains(low, "tls: bad certificate") {
		return diagnostic{
			headline: "TLS handshake failed for " + nonEmpty(host, "the upstream service"),
			body:     "The TLS layer rejected the connection before getting to the application protocol.",
			fix:      "Often a corporate HTTPS proxy or an outdated CA bundle. Try the command on a different network to isolate the cause.",
		}, true
	}
	return diagnostic{}, false
}

// classifyNet handles DNS / timeout / refused / unreachable. Typed
// (net.DNSError, net.OpError, syscall.ECONNREFUSED etc.) first, then
// substring fallback for stringified chains (copy-pasted reports).
func classifyNet(err error) (diagnostic, bool) {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsDiag(dnsErr.Name), true
	}
	// Refused precedes timeout so we give a sharper diagnosis.
	if errors.Is(err, syscall.ECONNREFUSED) {
		return refusedDiag(err), true
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		host := extractHost(err)
		return diagnostic{
			headline: "No route to " + nonEmpty(host, "the upstream service"),
			body:     "Your machine has no route at all to reach that IP. Often a wifi or VPN that's only half-up.",
			fix:      "Toggle wifi / VPN. If you're on a corporate VPN, confirm split-tunnel rules let Microsoft endpoints out.",
		}, true
	}
	if isTimeoutErr(err) {
		return timeoutDiag(err), true
	}

	// Substring fallback for chains where %v broke the typed unwrap.
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "no such host"):
		return dnsDiag(lastURLHost(err.Error())), true
	case strings.Contains(low, "connection refused"), strings.Contains(low, "actively refused"):
		return refusedDiag(err), true
	case isNetTimeoutString(low):
		return timeoutDiag(err), true
	}
	return diagnostic{}, false
}

// dnsDiag is shared by the typed (net.DNSError) and substring DNS paths.
func dnsDiag(host string) diagnostic {
	return diagnostic{
		headline: "DNS lookup failed for " + nonEmpty(host, "the upstream host"),
		body:     "Your system couldn't resolve the hostname to an IP. The tool never got far enough to make a network request.",
		causes: []string{
			"Captive portal (hotel/cafe wifi that hasn't been signed into yet)",
			"DNS server outage or misconfigured resolver",
			"VPN with broken split-DNS",
		},
		fix: "Try `nslookup " + nonEmpty(host, "device.auth.xboxlive.com") + "`.\n" +
			"If it also fails, switch DNS to 1.1.1.1 or 8.8.8.8 and retry.",
	}
}

// refusedDiag is shared by the typed (ECONNREFUSED) and substring refused paths.
func refusedDiag(err error) diagnostic {
	host := extractHost(err)
	if game := firstMatch(reGameServer, err.Error()); game != "" {
		return diagnostic{
			headline: game + " refused the connection",
			body:     "The host is reachable but nothing's listening on that port. Usually means the server is offline, the port is wrong, or you've been IP-banned.",
			fix: "Confirm the address is right (default Bedrock port is 19132).\n" +
				"If you can join from the in-game Servers list but not via this tool, your IP may be banned at the network level.",
		}
	}
	return diagnostic{
		headline: "Connection refused by " + nonEmpty(host, "the upstream service"),
		body:     "The host is reachable but actively refused our connection. For Microsoft / Mojang services this is almost always a temporary outage on their side.",
		fix:      "Wait a few minutes and retry. If it persists for more than ~30 min, check https://xnotify.xboxlive.com/servicestatus.",
	}
}

// appLayerKickDiag builds the diagnostic for a server-side kick that
// happened after gophertunnel finished its handshake. game is the
// `connection to X failed:` host:port; msg is the full error chain.
// The verbatim kick string from the server is included in the body
// when we can recover it.
func appLayerKickDiag(game, msg string) diagnostic {
	body := "RakNet handshake and Xbox sign-in both succeeded. The server's app layer rejected the session afterwards - typical anti-bot heuristic on big Bedrock networks."
	if kick := extractServerKickMessage(msg); kick != "" {
		body += "\n\nReason returned by the server:\n  " + strings.ReplaceAll(kick, "\n", "\n  ")
	}
	return diagnostic{
		headline: game + " kicked us after the handshake",
		body:     body,
		fix:      "Many large Bedrock partner servers run anti-bot heuristics that reject any client whose packet fingerprint doesn't match the official client. There's no general workaround from the tool side - those servers don't want third-party clients through.",
	}
}

// extractServerKickMessage pulls the verbatim kick string out of a
// gophertunnel `dial minecraft <local>->[remote]: <reason>` chain.
// Returns "" when the chain doesn't carry one. Trims the trailing
// `... minecraft <addr>->...: use of closed network connection` that
// gophertunnel appends after the kick text.
func extractServerKickMessage(msg string) string {
	i := strings.Index(msg, "dial minecraft ")
	if i < 0 {
		return ""
	}
	// First ": " after "dial minecraft " is the boundary between the
	// addr pair and the wrapped error. The colons inside <host:port>
	// have no trailing space so they're skipped.
	rest := msg[i+len("dial minecraft "):]
	sep := strings.Index(rest, ": ")
	if sep < 0 {
		return ""
	}
	kick := rest[sep+2:]
	// Strip gophertunnel's trailer if it's present. Use a regex rather
	// than plain Contains(" minecraft ") so a kick string containing
	// "minecraft" as a word isn't truncated.
	if loc := reKickTrailer.FindStringIndex(kick); loc != nil {
		kick = kick[:loc[0]]
	}
	return strings.TrimSpace(kick)
}

var reKickTrailer = regexp.MustCompile(`\s*minecraft \S+->\S+:\s*use of closed network connection\s*$`)

// timeoutDiag builds the "couldn't reach X" output. Typed and
// substring timeout paths share this so the rendered diagnostic
// is identical regardless of how the err arrived.
func timeoutDiag(err error) diagnostic {
	host := extractHost(err)
	game := firstMatch(reGameServer, err.Error())
	if game != "" && !strings.Contains(host, "xboxlive") && !strings.Contains(host, "playfab") {
		return diagnostic{
			headline: "Couldn't reach the Bedrock server at " + game,
			body:     "Connection timed out at the RakNet layer. Either the server is offline, the address is wrong, or something between you and it is dropping UDP.",
			causes: []string{
				"Server is offline or restarting",
				"Wrong host / port (default is 19132)",
				"ISP / firewall blocks outbound UDP",
			},
			fix: "Try the same address in the in-game Servers tab.\n" +
				"If the game can connect but the tool can't, check whether anything filters UDP on your network.",
		}
	}
	if isMicrosoftHost(host) {
		return diagnostic{
			headline: "Couldn't reach " + friendlyService(host),
			body:     host + " isn't responding from your network.",
			causes: []string{
				"ISP-level blocking of Xbox / Microsoft services in your region",
				"Corporate, school, or hotel firewall",
				"Aggressive antivirus or \"Family Safety\" software blocking xboxlive.com",
				"VPN required by your network (rare but seen)",
			},
			fix: "Quick check: `curl -v https://" + host + "/`\n" +
				"If curl also hangs, it's your network - use a VPN to a region where Xbox Live works (most EU/US locations are fine).",
		}
	}
	// Unknown host: skip the curl-against-xboxlive hint - it could be
	// a CDN or third party where the Xbox-VPN advice would be wrong.
	target := nonEmpty(host, "the upstream service")
	return diagnostic{
		headline: "Couldn't reach " + target,
		body:     "The request timed out without reaching the server.",
		causes: []string{
			"The server is offline or overloaded",
			"Your network blocks the destination (firewall, ISP, VPN split-tunnel)",
		},
		fix: "Try again in a few minutes. If it keeps failing, capture the chain in Details below and confirm reachability with curl or your browser.",
	}
}

// isNetTimeoutString catches timeout phrasing when the typed surface
// has been erased. Refused has its own separate branch.
func isNetTimeoutString(low string) bool {
	switch {
	case strings.Contains(low, "i/o timeout"),
		strings.Contains(low, "connect: connection timed out"),
		strings.Contains(low, "connectex: a connection attempt failed"),
		strings.Contains(low, "no route to host"),
		strings.Contains(low, "network is unreachable"),
		strings.Contains(low, "host is down"),
		strings.Contains(low, "context deadline exceeded") && strings.Contains(low, "dial"):
		return true
	}
	return false
}

// lastURLHost returns the host of the innermost URL in msg (chains
// read outer-to-inner, so the last match is the one that failed).
func lastURLHost(msg string) string {
	all := reURLHost.FindAllStringSubmatch(msg, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1][1]
}

// classifyXSTS catches Xbox Live account-state failures. gophertunnel
// surfaces them only in the message string, so substring is the only
// option. Both hex (0x8015dcXX) and decimal forms are matched.
func classifyXSTS(msg, low string) (diagnostic, bool) {
	hasAuthCtx := strings.Contains(msg, "xbox auth") || strings.Contains(msg, "xsts") || strings.Contains(low, "xerr")
	if !hasAuthCtx {
		return diagnostic{}, false
	}

	switch {
	case containsAny(msg, "2148916227", "8015dc03", "Identity ban"),
		containsAny(low, "account banned"):
		return diagnostic{
			headline: "This Microsoft account is banned from Xbox Live",
			body:     "Microsoft's identity service rejected the sign-in with an account-ban code. We can't work around it from the tool.",
			fix:      "Use a different Microsoft account. Bans are a Microsoft policy decision, appealable only via account.microsoft.com.",
		}, true

	case containsAny(msg, "2148916233", "8015dc09"),
		containsAny(low, "no xbox account", "create an xbox account", "xbox live profile"):
		return diagnostic{
			headline: "This Microsoft account doesn't have an Xbox profile yet",
			body:     "Every MSA needs to be associated with an Xbox gamertag once before the franchise chain works.",
			fix: "Open xbox.com and sign in with this MSA to create the profile.\n" +
				"Once the gamertag prompt completes, re-run the same command - the tool will pick up the existing token.",
		}, true

	case containsAny(msg, "2148916235", "8015dc0b"),
		containsAny(low, "country", "region"):
		return diagnostic{
			headline: "This account's region doesn't allow Xbox Live",
			body:     "The Microsoft account's set country is one where Xbox Live isn't operated (or is sanctioned).",
			fix: "Change the account region at account.microsoft.com/profile to one Xbox Live supports, then retry.\n" +
				"Or use a different MSA that's already in a supported region.",
		}, true

	case containsAny(msg, "2148916236", "2148916237", "2148916238", "8015dc0c", "8015dc0d", "8015dc0e"),
		containsAny(low, "parental consent", "child account", "adult"):
		return diagnostic{
			headline: "Xbox Live needs parental consent for this account",
			body:     "The account is flagged as a child / family member and a parent hasn't approved Xbox Live access yet.",
			fix:      "Sign in to xbox.com as the family organiser and approve Xbox Live for this account, then retry.",
		}, true

	case strings.Contains(msg, "xbox auth"):
		return diagnostic{
			headline: "Microsoft sign-in failed during the Xbox handshake",
			body:     "The MSA -> Xbox Live -> PlayFab chain rejected the credentials with a non-network error.",
			fix:      "Re-running often clears transient hiccups. If it persists, delete the cached token at `" + tokenPath() + "` and authenticate from scratch.",
		}, true
	}
	return diagnostic{}, false
}

var (
	reGameServer = regexp.MustCompile(`connection to (\S+) failed:`)
	reURLHost    = regexp.MustCompile(`https?://([^/\s")]+)`)
)

// extractHost pulls a hostname out of err using typed sources first
// (url.Error.URL, net.OpError.Addr), then falls back to scraping the
// message for the innermost URL.
func extractHost(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if u, perr := url.Parse(urlErr.URL); perr == nil && u.Host != "" {
			return u.Hostname()
		}
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Addr != nil {
		// Addr.String() is "host:port" for TCP/UDP, bare "ip" for IPAddr.
		s := opErr.Addr.String()
		if h, _, splitErr := net.SplitHostPort(s); splitErr == nil {
			return h
		}
		return s
	}
	return lastURLHost(err.Error())
}

func firstMatch(re *regexp.Regexp, msg string) string {
	m := re.FindStringSubmatch(msg)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func nonEmpty(a, fallback string) string {
	if a == "" {
		return fallback
	}
	return a
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// isTimeoutErr is true for any chain element that reports Timeout().
// context.DeadlineExceeded satisfies net.Error so it's caught here too.
func isTimeoutErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isMicrosoftHost reports whether host is one of the franchise
// endpoints, gating the "curl xboxlive + VPN" hint in timeoutDiag.
func isMicrosoftHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(host)
	return strings.Contains(h, "xboxlive.com") ||
		strings.Contains(h, "playfab") ||
		strings.Contains(h, "minecraft-services") ||
		strings.Contains(h, "minecraftservices") ||
		strings.Contains(h, "login.live.com") ||
		strings.Contains(h, "microsoftonline")
}

// friendlyService maps a hostname to a consumer-facing service name.
func friendlyService(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "xboxlive.com"):
		return "Xbox Live"
	case strings.Contains(h, "playfab"):
		return "PlayFab"
	case strings.Contains(h, "minecraft-services") || strings.Contains(h, "minecraftservices"):
		return "Mojang's franchise services"
	case strings.Contains(h, "login.live.com") || strings.Contains(h, "microsoftonline"):
		return "Microsoft sign-in"
	case h != "":
		return host
	}
	return "the upstream service"
}

// mustMCTokenPath is the path for "rm <token>" instructions, with a
// readable placeholder when UserConfigDir fails.
func mustMCTokenPath() string {
	p, err := mctokenPath()
	if err != nil {
		return "<cache dir>/.mctoken.json"
	}
	return p
}

// writeDiagnostic renders a diagnostic plus the raw chain under
// "Details:" so bug reports keep full provenance.
func writeDiagnostic(w io.Writer, d diagnostic, raw error) {
	fmt.Fprintf(w, "\n  %sError: %s%s\n\n", colorRed, d.headline, colorReset)
	if d.body != "" {
		writeIndented(w, d.body, "    ")
	}
	if len(d.causes) > 0 {
		fmt.Fprintf(w, "\n    Common causes:\n")
		for _, c := range d.causes {
			fmt.Fprintf(w, "      - %s\n", c)
		}
	}
	if d.fix != "" {
		fmt.Fprintln(w)
		writeIndented(w, d.fix, "    ")
	}
	fmt.Fprintf(w, "\n  %sDetails (paste into bug reports):%s\n", colorYellow, colorReset)
	writeIndented(w, raw.Error(), "    ")
	fmt.Fprintln(w)
}

// writeRawError keeps unmatched errors as a single colored line.
func writeRawError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n  %sError: %v%s\n", colorRed, err, colorReset)
}

func writeIndented(w io.Writer, body, prefix string) {
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		fmt.Fprintf(w, "%s%s\n", prefix, line)
	}
}
