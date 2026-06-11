package main

import (
	"cmp"
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
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
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
			headline: lang.T("humanize.discovery.headline"),
			body:     lang.T("humanize.discovery.body"),
			fix:      lang.T("humanize.discovery.fix"),
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
				headline: lang.Tf("humanize.protocol.headline", game),
				body:     lang.T("humanize.protocol.body"),
				fix:      lang.T("humanize.protocol.fix"),
			}, true
		case containsAny(low, "kicked", "disconnect", "banned"):
			return diagnostic{
				headline: lang.Tf("humanize.kick.headline", game),
				body:     lang.T("humanize.kick.body"),
				fix:      lang.T("humanize.kick.fix"),
			}, true
		}
		return diagnostic{
			headline: lang.Tf("humanize.raknet.headline", game),
			body:     lang.T("humanize.raknet.body"),
			fix:      lang.T("humanize.raknet.fix"),
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
			headline: lang.T("humanize.venue.headline"),
			body:     lang.T("humanize.venue.body"),
			fix:      lang.T("humanize.venue.fix"),
		}, true

	case errors.Is(err, franchise.ErrForbidden):
		return diagnostic{
			headline: lang.T("humanize.forbidden.headline"),
			body:     lang.T("humanize.forbidden.body"),
			fix:      lang.T("humanize.forbidden.fix"),
		}, true

	case errors.Is(err, franchise.ErrAuthRejected):
		// Reaching the user means the featured.go re-mint retry also failed.
		return diagnostic{
			headline: lang.T("humanize.authrejected.headline"),
			body:     lang.T("humanize.authrejected.body"),
			fix:      lang.Tf("humanize.authrejected.fix", mustTokenPath(), mustMCTokenPath()),
		}, true

	case errors.Is(err, errPackNoManifest):
		return diagnostic{
			headline: lang.T("humanize.nomanifest.headline"),
			body:     lang.T("humanize.nomanifest.body"),
			fix:      lang.T("humanize.nomanifest.fix"),
		}, true

	case errors.Is(err, errPackBadManifest):
		return diagnostic{
			headline: lang.T("humanize.badmanifest.headline"),
			body:     lang.T("humanize.badmanifest.body"),
			fix:      lang.T("humanize.badmanifest.fix"),
		}, true

	case errors.Is(err, errPackBadKeyLen):
		return diagnostic{
			headline: lang.T("humanize.badkeylen.headline"),
			body:     lang.T("humanize.badkeylen.body"),
			fix:      lang.T("humanize.badkeylen.fix"),
		}, true

	case errors.Is(err, errPackWrongKey):
		return diagnostic{
			headline: lang.T("humanize.wrongkey.headline"),
			body:     lang.T("humanize.wrongkey.body"),
			fix:      lang.T("humanize.wrongkey.fix"),
		}, true

	case errors.Is(err, errPackTruncated):
		return diagnostic{
			headline: lang.T("humanize.truncated.headline"),
			body:     lang.T("humanize.truncated.body"),
			fix:      lang.T("humanize.truncated.fix"),
		}, true

	case errors.Is(err, errPackBadProtocol):
		return diagnostic{
			headline: lang.T("humanize.badprotocol.headline"),
			body:     lang.T("humanize.badprotocol.body"),
			fix:      lang.T("humanize.badprotocol.fix"),
		}, true

	case errors.Is(err, errPackBadZip):
		return diagnostic{
			headline: lang.T("humanize.badzip.headline"),
			body:     lang.T("humanize.badzip.body"),
			fix:      lang.T("humanize.badzip.fix"),
		}, true

	case errors.Is(err, errPackEmpty):
		return diagnostic{
			headline: lang.T("humanize.empty.headline"),
			body:     lang.T("humanize.empty.body"),
			fix:      lang.T("humanize.empty.fix"),
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
			headline: lang.T("humanize.oauthtiming.headline"),
			body:     lang.T("humanize.oauthtiming.body"),
			fix:      lang.T("humanize.oauthtiming.fix"),
		}, true
	case "invalid_grant":
		return diagnostic{
			headline: lang.T("humanize.oauthgrant.headline"),
			body:     lang.Tf("humanize.oauthgrant.body", mustTokenPath()),
			fix:      lang.Tf("humanize.oauthgrant.fix", mustTokenPath()),
		}, true
	}
	return diagnostic{
		headline: lang.Tf("humanize.oauthgeneric.headline", rErr.ErrorCode),
		body:     lang.T("humanize.oauthgeneric.body"),
		fix:      lang.Tf("humanize.oauthgeneric.fix", mustTokenPath()),
	}, true
}

// classifyFS catches permission / no-space / read-only. fs.ErrPermission
// matches EACCES/EPERM cross-platform without hard-coding errno.
func classifyFS(err error) (diagnostic, bool) {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return diagnostic{
			headline: lang.T("humanize.fsperm.headline"),
			body:     lang.T("humanize.fsperm.body"),
			fix:      lang.T("humanize.fsperm.fix"),
		}, true
	case errors.Is(err, syscall.ENOSPC):
		return diagnostic{
			headline: lang.T("humanize.fsnospace.headline"),
			body:     lang.T("humanize.fsnospace.body"),
			fix:      lang.T("humanize.fsnospace.fix"),
		}, true
	case errors.Is(err, syscall.EROFS):
		return diagnostic{
			headline: lang.T("humanize.fsrofs.headline"),
			body:     lang.T("humanize.fsrofs.body"),
			fix:      lang.T("humanize.fsrofs.fix"),
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
			headline: lang.Tf("humanize.tlsunknownca.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
			body:     lang.T("humanize.tlsunknownca.body"),
			causes:   lang.Tlist("humanize.tlsunknownca.causes"),
			fix:      lang.T("humanize.tlsunknownca.fix"),
		}, true
	}
	var ci x509.CertificateInvalidError
	if errors.As(err, &ci) {
		if ci.Reason == x509.Expired {
			return diagnostic{
				headline: lang.Tf("humanize.tlsexpired.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
				body:     lang.T("humanize.tlsexpired.body"),
				fix:      lang.T("humanize.tlsexpired.fix"),
			}, true
		}
		return diagnostic{
			headline: lang.Tf("humanize.tlsinvalid.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
			body:     lang.Tf("humanize.tlsinvalid.body", ci.Error()),
			fix:      lang.Tf("humanize.tlsinvalid.fix", cmp.Or(host, lang.T("humanize.placeholder.host"))),
		}, true
	}
	// Substring fallback for stringified TLS errors.
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "tls: handshake failure") || strings.Contains(low, "tls: bad certificate") {
		return diagnostic{
			headline: lang.Tf("humanize.tlshandshake.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
			body:     lang.T("humanize.tlshandshake.body"),
			fix:      lang.T("humanize.tlshandshake.fix"),
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
			headline: lang.Tf("humanize.noroute.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
			body:     lang.T("humanize.noroute.body"),
			fix:      lang.T("humanize.noroute.fix"),
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
		headline: lang.Tf("humanize.dns.headline", cmp.Or(host, lang.T("humanize.placeholder.upstreamhost"))),
		body:     lang.T("humanize.dns.body"),
		causes:   lang.Tlist("humanize.dns.causes"),
		fix:      lang.Tf("humanize.dns.fix", cmp.Or(host, lang.T("humanize.placeholder.dnshost"))),
	}
}

// refusedDiag is shared by the typed (ECONNREFUSED) and substring refused paths.
func refusedDiag(err error) diagnostic {
	host := extractHost(err)
	if game := firstMatch(reGameServer, err.Error()); game != "" {
		return diagnostic{
			headline: lang.Tf("humanize.refusedgame.headline", game),
			body:     lang.T("humanize.refusedgame.body"),
			fix:      lang.T("humanize.refusedgame.fix"),
		}
	}
	return diagnostic{
		headline: lang.Tf("humanize.refused.headline", cmp.Or(host, lang.T("humanize.placeholder.upstream"))),
		body:     lang.T("humanize.refused.body"),
		fix:      lang.T("humanize.refused.fix"),
	}
}

// appLayerKickDiag builds the diagnostic for a server-side kick that
// happened after gophertunnel finished its handshake. game is the
// `connection to X failed:` host:port; msg is the full error chain.
// The verbatim kick string from the server is included in the body
// when we can recover it.
func appLayerKickDiag(game, msg string) diagnostic {
	body := lang.T("humanize.applayerkick.bodybase")
	if kick := extractServerKickMessage(msg); kick != "" {
		body += lang.Tf("humanize.applayerkick.bodyreason", strings.ReplaceAll(kick, "\n", "\n  "))
	}
	return diagnostic{
		headline: lang.Tf("humanize.applayerkick.headline", game),
		body:     body,
		fix:      lang.T("humanize.applayerkick.fix"),
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
			headline: lang.Tf("humanize.timeoutgame.headline", game),
			body:     lang.T("humanize.timeoutgame.body"),
			causes:   lang.Tlist("humanize.timeoutgame.causes"),
			fix:      lang.T("humanize.timeoutgame.fix"),
		}
	}
	if isMicrosoftHost(host) {
		return diagnostic{
			headline: lang.Tf("humanize.timeoutms.headline", friendlyService(host)),
			body:     lang.Tf("humanize.timeoutms.body", host),
			causes:   lang.Tlist("humanize.timeoutms.causes"),
			fix:      lang.Tf("humanize.timeoutms.fix", host),
		}
	}
	// Unknown host: skip the curl-against-xboxlive hint - it could be
	// a CDN or third party where the Xbox-VPN advice would be wrong.
	target := cmp.Or(host, lang.T("humanize.placeholder.upstream"))
	return diagnostic{
		headline: lang.Tf("humanize.timeout.headline", target),
		body:     lang.T("humanize.timeout.body"),
		causes:   lang.Tlist("humanize.timeout.causes"),
		fix:      lang.T("humanize.timeout.fix"),
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
			headline: lang.T("humanize.xstsbanned.headline"),
			body:     lang.T("humanize.xstsbanned.body"),
			fix:      lang.T("humanize.xstsbanned.fix"),
		}, true

	case containsAny(msg, "2148916233", "8015dc09"),
		containsAny(low, "no xbox account", "create an xbox account", "xbox live profile", "does not have an xbox profile", "signup.live.com"):
		return diagnostic{
			headline: lang.T("humanize.xstsnoprofile.headline"),
			body:     lang.T("humanize.xstsnoprofile.body"),
			fix:      lang.T("humanize.xstsnoprofile.fix"),
		}, true

	case containsAny(msg, "2148916235", "8015dc0b"),
		containsAny(low, "country", "region"):
		return diagnostic{
			headline: lang.T("humanize.xstsregion.headline"),
			body:     lang.T("humanize.xstsregion.body"),
			fix:      lang.T("humanize.xstsregion.fix"),
		}, true

	case containsAny(msg, "2148916236", "2148916237", "2148916238", "8015dc0c", "8015dc0d", "8015dc0e"),
		containsAny(low, "parental consent", "child account", "adult"):
		return diagnostic{
			headline: lang.T("humanize.xstschild.headline"),
			body:     lang.T("humanize.xstschild.body"),
			fix:      lang.T("humanize.xstschild.fix"),
		}, true

	case strings.Contains(msg, "xbox auth"):
		return diagnostic{
			headline: lang.T("humanize.xstsgeneric.headline"),
			body:     lang.T("humanize.xstsgeneric.body"),
			fix:      lang.Tf("humanize.xstsgeneric.fix", mustTokenPath()),
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
		return lang.T("humanize.service.mojang")
	case strings.Contains(h, "login.live.com") || strings.Contains(h, "microsoftonline"):
		return lang.T("humanize.service.mssignin")
	case h != "":
		return host
	}
	return lang.T("humanize.placeholder.upstream")
}

// mustTokenPath is the path for "rm <token>" instructions, with a
// readable placeholder when UserConfigDir fails.
func mustTokenPath() string {
	p, err := tokenPath()
	if err != nil {
		return "<cache dir>/" + tokenFileName
	}
	return p
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
	fmt.Fprintf(w, "\n  %s%s%s%s\n\n", colorRed, lang.T("humanize.render.errorprefix"), d.headline, colorReset)
	if d.body != "" {
		writeIndented(w, d.body, "    ")
	}
	if len(d.causes) > 0 {
		fmt.Fprintf(w, "\n    %s\n", lang.T("humanize.render.causeslabel"))
		for _, c := range d.causes {
			fmt.Fprintf(w, "      - %s\n", c)
		}
	}
	if d.fix != "" {
		fmt.Fprintln(w)
		writeIndented(w, d.fix, "    ")
	}
	fmt.Fprintf(w, "\n  %s%s%s\n", colorYellow, lang.T("humanize.render.detailslabel"), colorReset)
	writeIndented(w, raw.Error(), "    ")
	fmt.Fprintln(w)
}

// writeRawError keeps unmatched errors as a single colored line.
func writeRawError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n  %s%s%v%s\n", colorRed, lang.T("humanize.render.errorprefix"), err, colorReset)
}

func writeIndented(w io.Writer, body, prefix string) {
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		fmt.Fprintf(w, "%s%s\n", prefix, line)
	}
}
