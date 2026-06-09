package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"strings"
	"syscall"
	"testing"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"golang.org/x/oauth2"
)

// realWorldJamesError is the exact chain reported by the user JamesPhilpot
// on 2026-05-29 - a Windows ISP-blocked TCP connection to xboxlive.com.
// Verbatim copy is the only reliable test we won't regress against.
const realWorldJamesError = `mint mctoken: login playfab: request xbox live token: request xsts token for "http://playfab.xboxlive.com/": request device token: obtain device token: POST https://device.auth.xboxlive.com/device/authenticate: Post "https://device.auth.xboxlive.com/device/authenticate": dial tcp 40.90.8.153:443: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond.`

// fakeNetTimeout fabricates a *net.OpError that reports Timeout()=true,
// matching what gophertunnel's HTTP client surfaces on a real timeout.
// We can't fake the rest of the wrapping chain typewise, so we wrap it
// in a url.Error like net/http would.
func fakeNetTimeoutErr(host string) error {
	addr := &net.TCPAddr{IP: net.ParseIP("40.90.8.153"), Port: 443}
	op := &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: addr,
		Err:  &timeoutErr{},
	}
	return &url.Error{Op: "Post", URL: "https://" + host + "/device/authenticate", Err: op}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

// fakeRefused fabricates a net.OpError carrying syscall.ECONNREFUSED so
// errors.Is(err, syscall.ECONNREFUSED) lights up.
func fakeRefusedErr() error {
	op := &net.OpError{
		Op:   "dial",
		Net:  "tcp",
		Addr: &net.TCPAddr{IP: net.ParseIP("40.90.8.153"), Port: 443},
		Err:  &refusedErr{},
	}
	return &url.Error{Op: "Post", URL: "https://device.auth.xboxlive.com/device/authenticate", Err: op}
}

type refusedErr struct{}

func (refusedErr) Error() string           { return "connection refused" }
func (e *refusedErr) Is(target error) bool { return target == syscall.ECONNREFUSED }
func (refusedErr) Timeout() bool           { return false }
func (refusedErr) Temporary() bool         { return false }
func (refusedErr) Unwrap() error           { return syscall.ECONNREFUSED }

// fakeDNSErr fabricates a *net.DNSError covering the typical NXDOMAIN
// path that errors.As against *net.DNSError can pick up.
func fakeDNSErr() error {
	return &net.DNSError{
		Err:        "no such host",
		Name:       "device.auth.xboxlive.com",
		IsNotFound: true,
	}
}

func TestHumanize(t *testing.T) {
	type tc struct {
		name             string
		err              error
		wantOK           bool
		wantHeadlinePart string
		wantBodyPart     string
		wantFixPart      string
	}
	cases := []tc{
		{
			name:             "real-world James timeout (Windows connectex)",
			err:              errors.New(realWorldJamesError),
			wantOK:           true,
			wantHeadlinePart: "Couldn't reach Xbox Live",
			wantBodyPart:     "device.auth.xboxlive.com",
			wantFixPart:      "curl -v",
		},
		{
			name:             "typed net.OpError timeout via url.Error",
			err:              fakeNetTimeoutErr("device.auth.xboxlive.com"),
			wantOK:           true,
			wantHeadlinePart: "Couldn't reach Xbox Live",
			wantBodyPart:     "device.auth.xboxlive.com",
		},
		{
			name:             "typed ECONNREFUSED",
			err:              fakeRefusedErr(),
			wantOK:           true,
			wantHeadlinePart: "Connection refused",
		},
		{
			name:             "typed net.DNSError",
			err:              fakeDNSErr(),
			wantOK:           true,
			wantHeadlinePart: "DNS lookup failed",
			wantBodyPart:     "couldn't resolve",
		},
		{
			name:             "typed x509 UnknownAuthorityError",
			err:              &url.Error{Op: "Get", URL: "https://playfab.xboxlive.com/", Err: x509.UnknownAuthorityError{}},
			wantOK:           true,
			wantHeadlinePart: "TLS handshake failed",
			wantBodyPart:     "HTTPS-inspecting proxy",
		},
		{
			name:             "typed x509 expired certificate",
			err:              x509.CertificateInvalidError{Reason: x509.Expired},
			wantOK:           true,
			wantHeadlinePart: "TLS certificate looks expired",
			wantFixPart:      "system clock",
		},
		{
			name:             "typed oauth2 RetrieveError: authorization_pending",
			err:              &oauth2.RetrieveError{ErrorCode: "authorization_pending"},
			wantOK:           true,
			wantHeadlinePart: "Microsoft sign-in didn't complete",
		},
		{
			name:             "typed oauth2 RetrieveError: invalid_grant",
			err:              fmt.Errorf("xbox auth: %w", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}),
			wantOK:           true,
			wantHeadlinePart: "refresh token is no longer valid",
		},
		{
			name:             "typed fs.ErrPermission",
			err:              fmt.Errorf("create output dir: %w", fs.ErrPermission),
			wantOK:           true,
			wantHeadlinePart: "Permission denied",
		},
		{
			name:             "typed syscall.ENOSPC",
			err:              fmt.Errorf("write key file: %w", syscall.ENOSPC),
			wantOK:           true,
			wantHeadlinePart: "Disk is full",
		},
		{
			name:             "typed syscall.EROFS",
			err:              fmt.Errorf("create output dir: %w", syscall.EROFS),
			wantOK:           true,
			wantHeadlinePart: "read-only filesystem",
		},
		{
			name:             "typed context.DeadlineExceeded",
			err:              fmt.Errorf("franchise request: %w", context.DeadlineExceeded),
			wantOK:           true,
			wantHeadlinePart: "Couldn't reach",
		},
		{
			name:             "our sentinel: franchise.ErrExperienceOffline",
			err:              fmt.Errorf("resolve experience %q: %w", "Some Slot", franchise.ErrExperienceOffline),
			wantOK:           true,
			wantHeadlinePart: "no active venue",
		},
		{
			name:             "our sentinel: franchise.ErrAuthRejected (twice-failed)",
			err:              fmt.Errorf("featured catalog: %w", franchise.ErrAuthRejected),
			wantOK:           true,
			wantHeadlinePart: "Xbox identity was rejected",
			wantFixPart:      "rm ",
		},
		{
			name:             "our sentinel: franchise.ErrForbidden (403, not a token problem)",
			err:              fmt.Errorf("resolve experience %q: %w", "X", franchise.ErrForbidden),
			wantOK:           true,
			wantHeadlinePart: "won't let this account",
			wantFixPart:      "different entry",
		},
		{
			name:             "our sentinel: errPackNoManifest",
			err:              fmt.Errorf("%w: no header.uuid", errPackNoManifest),
			wantOK:           true,
			wantHeadlinePart: "isn't a valid resource pack",
		},
		{
			name:             "our sentinel: errPackBadKeyLen",
			err:              fmt.Errorf("%w: got 16 characters", errPackBadKeyLen),
			wantOK:           true,
			wantHeadlinePart: "Key length is wrong",
		},
		{
			name:             "our sentinel: errPackWrongKey",
			err:              fmt.Errorf("%w: cipher: message authentication failed", errPackWrongKey),
			wantOK:           true,
			wantHeadlinePart: "Decryption failed",
		},
		{
			name:             "our sentinel: errPackTruncated",
			err:              fmt.Errorf("%w: 12 bytes", errPackTruncated),
			wantOK:           true,
			wantHeadlinePart: "contents.json truncated",
		},
		{
			name:             "our sentinel: errPackBadProtocol",
			err:              fmt.Errorf("%w: payload: bad shape", errPackBadProtocol),
			wantOK:           true,
			wantHeadlinePart: "unexpected pack-info payload",
		},
		{
			name:             "our sentinel: errPackBadZip",
			err:              fmt.Errorf("%w: zip: unexpected EOF", errPackBadZip),
			wantOK:           true,
			wantHeadlinePart: "Pack download was incomplete",
		},
		{
			name:             "our sentinel: errPackEmpty",
			err:              fmt.Errorf("%w (only 2 non-encryptable files)", errPackEmpty),
			wantOK:           true,
			wantHeadlinePart: "nothing to encrypt",
		},
		{
			name:             "XSTS: no Xbox profile (decimal code)",
			err:              errors.New("xbox auth: request xsts token: 2148916233"),
			wantOK:           true,
			wantHeadlinePart: "doesn't have an Xbox profile",
		},
		{
			name:             "XSTS: no Xbox profile (hex code)",
			err:              errors.New("xbox auth: xsts XErr=0x8015dc09"),
			wantOK:           true,
			wantHeadlinePart: "doesn't have an Xbox profile",
		},
		{
			name:             "XSTS: account banned",
			err:              errors.New("xbox auth: xsts Identity ban"),
			wantOK:           true,
			wantHeadlinePart: "banned from Xbox Live",
		},
		{
			name:             "XSTS: region locked",
			err:              errors.New("xbox auth: XErr=2148916235 country"),
			wantOK:           true,
			wantHeadlinePart: "region doesn't allow Xbox Live",
		},
		{
			name:             "XSTS: parental consent",
			err:              errors.New("xbox auth: XErr=2148916236"),
			wantOK:           true,
			wantHeadlinePart: "parental consent",
		},
		{
			name:             "XSTS: generic xbox auth fallback",
			err:              errors.New("xbox auth: something unspecific went wrong"),
			wantOK:           true,
			wantHeadlinePart: "Microsoft sign-in failed",
		},
		{
			name:             "discovery failure",
			err:              errors.New("discover services: some-error"),
			wantOK:           true,
			wantHeadlinePart: "service discovery failed",
		},
		{
			name:             "game server: bare connection wrap (timeout-less)",
			err:              errors.New("connection to play.example.net:19132 failed: handshake aborted"),
			wantOK:           true,
			wantHeadlinePart: "Couldn't connect to play.example.net:19132",
		},
		{
			name:             "game server: protocol mismatch",
			err:              errors.New("connection to play.example.net:19132 failed: incompatible protocol"),
			wantOK:           true,
			wantHeadlinePart: "different protocol version",
		},
		{
			name:             "game server: kicked",
			err:              errors.New("connection to play.example.net:19132 failed: disconnect: kicked"),
			wantOK:           true,
			wantHeadlinePart: "kicked us",
		},
		{
			// Mirrors the real-world shape: `dial minecraft <addr>`
			// marker indicates RakNet + auth both passed before the
			// server-side kick.
			name:             "game server: app-layer kick after handshake",
			err:              errors.New("connection to play.example.net:19132 failed: dial minecraft 192.168.1.14:55799->203.0.113.7:19132: please join through the official Servers tab.\nThird-party clients are not supported. minecraft 192.168.1.14:55799->203.0.113.7:19132: use of closed network connection"),
			wantOK:           true,
			wantHeadlinePart: "kicked us after the handshake",
			wantBodyPart:     "please join through the official Servers tab",
			wantFixPart:      "anti-bot",
		},
		{
			// Some servers send anti-bot kicks whose text mentions
			// "outdated" / "protocol" as rhetoric. dial minecraft in
			// the chain proves handshake finished, so this is still an
			// app-layer kick - not the protocol-mismatch branch.
			name:             "game server: anti-bot kick mentioning 'outdated'",
			err:              errors.New("connection to play.example.net:19132 failed: dial minecraft 192.168.1.14:51888->203.0.113.7:19132: Outdated proxy! This server supports versions: 1.21.111, 1.21.130 minecraft 192.168.1.14:51888->203.0.113.7:19132: use of closed network connection"),
			wantOK:           true,
			wantHeadlinePart: "kicked us after the handshake",
			wantBodyPart:     "Outdated proxy",
			wantFixPart:      "anti-bot",
		},
		{
			name:   "unknown error falls through",
			err:    errors.New("something totally weird that we don't classify"),
			wantOK: false,
		},
		{
			name:   "nil err returns false",
			err:    nil,
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := humanize(c.err)
			if ok != c.wantOK {
				t.Fatalf("humanize ok=%v want %v\nheadline=%q\nfor err=%v", ok, c.wantOK, got.headline, c.err)
			}
			if !c.wantOK {
				return
			}
			if c.wantHeadlinePart != "" && !strings.Contains(got.headline, c.wantHeadlinePart) {
				t.Errorf("headline=%q does not contain %q", got.headline, c.wantHeadlinePart)
			}
			if c.wantBodyPart != "" && !strings.Contains(got.body, c.wantBodyPart) {
				t.Errorf("body=%q does not contain %q", got.body, c.wantBodyPart)
			}
			if c.wantFixPart != "" && !strings.Contains(got.fix, c.wantFixPart) {
				t.Errorf("fix=%q does not contain %q", got.fix, c.wantFixPart)
			}
		})
	}
}

// TestHumanizePriority verifies that more-specific classifiers win
// over the generic "network timeout" fallback when both could match.
// In particular, our sentinels and XSTS-code matches must beat any
// timeout/refused branch reachable through the same chain.
func TestHumanizePriority(t *testing.T) {
	// franchise.ErrAuthRejected wrapped in a chain that also mentions "timeout"
	// should still classify as the sentinel (account problem), not as
	// a generic timeout.
	err := fmt.Errorf("featured: %w: prior call timed out", franchise.ErrAuthRejected)
	d, ok := humanize(err)
	if !ok || !strings.Contains(d.headline, "Xbox identity was rejected") {
		t.Fatalf("sentinel priority lost: ok=%v headline=%q", ok, d.headline)
	}
}

// TestExtractHostFromURLError pins down that we prefer the typed URL
// over scraping the message - both should work, the typed path is
// what we want to exercise.
func TestExtractHostFromURLError(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://device.auth.xboxlive.com/x", Err: errors.New("boom")}
	got := extractHost(err)
	if got != "device.auth.xboxlive.com" {
		t.Errorf("extractHost = %q, want device.auth.xboxlive.com", got)
	}
}

func TestExtractServerKickMessage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "full chain with trailer",
			in:   `connection to play.example.net:19132 failed: dial minecraft 192.168.1.14:55799->203.0.113.7:19132: please join through the official Servers tab.` + "\n" + `Third-party clients are not supported. minecraft 192.168.1.14:55799->203.0.113.7:19132: use of closed network connection`,
			want: "please join through the official Servers tab.\nThird-party clients are not supported.",
		},
		{
			name: "no dial minecraft marker",
			in:   "connection to x:19132 failed: i/o timeout",
			want: "",
		},
		{
			name: "kick without trailer",
			in:   "dial minecraft 1.2.3.4:55799->5.6.7.8:19132: server is in maintenance",
			want: "server is in maintenance",
		},
		{
			name: "kick mentioning word minecraft",
			in:   "dial minecraft 1.2.3.4:55799->5.6.7.8:19132: please use the official Minecraft client. minecraft 1.2.3.4:55799->5.6.7.8:19132: use of closed network connection",
			want: "please use the official Minecraft client.",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractServerKickMessage(c.in); got != c.want {
				t.Errorf("extractServerKickMessage:\n  got:  %q\n  want: %q", got, c.want)
			}
		})
	}
}

// TestExtractHostFromOpErrorAddr verifies the net.OpError.Addr path
// when no url.Error is present in the chain (e.g. RakNet dial errors).
func TestExtractHostFromOpErrorAddr(t *testing.T) {
	op := &net.OpError{
		Op:   "dial",
		Net:  "udp",
		Addr: &net.UDPAddr{IP: net.ParseIP("40.90.8.153"), Port: 19132},
		Err:  errors.New("i/o timeout"),
	}
	got := extractHost(op)
	if got != "40.90.8.153" {
		t.Errorf("extractHost = %q, want 40.90.8.153", got)
	}
}

func TestFriendlyService(t *testing.T) {
	cases := []struct {
		host, want string
	}{
		{"device.auth.xboxlive.com", "Xbox Live"},
		{"playfab.xboxlive.com", "Xbox Live"},
		{"playfabapi.com", "PlayFab"},
		{"client.discovery.minecraft-services.net", "Mojang's franchise services"},
		{"minecraftservices.net", "Mojang's franchise services"},
		{"login.live.com", "Microsoft sign-in"},
		{"random.example.com", "random.example.com"},
		{"", "the upstream service"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if got := friendlyService(tc.host); got != tc.want {
				t.Errorf("friendlyService(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

func TestIsMicrosoftHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"", false},
		{"device.auth.xboxlive.com", true},
		{"playfab.xboxlive.com", true},
		{"playfabapi.com", true},
		{"client.discovery.minecraft-services.net", true},
		{"login.live.com", true},
		{"cdn.example.com", false},
		{"play.example.net", false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if got := isMicrosoftHost(tc.host); got != tc.want {
				t.Errorf("isMicrosoftHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestLastURLHost(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"single URL", `Post "https://a.example/x":`, "a.example"},
		{"prefer last (innermost)", `XYZ "https://outer.example" -> "https://inner.example":`, "inner.example"},
		{"James chain takes innermost", realWorldJamesError, "device.auth.xboxlive.com"},
		{"no URL at all", "just some error string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastURLHost(tc.in); got != tc.want {
				t.Errorf("lastURLHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMustMCTokenPath(t *testing.T) {
	// We can't easily fake os.UserConfigDir, but we can assert the
	// successful return has the .mctoken.json suffix and is not the
	// placeholder. On the rare CI runner without a config dir, the
	// placeholder is acceptable.
	got := mustMCTokenPath()
	if got == "" {
		t.Fatal("mustMCTokenPath returned empty string")
	}
	if got != "<cache dir>/.mctoken.json" && !strings.HasSuffix(got, mctokenFileName) {
		t.Errorf("mustMCTokenPath = %q, want suffix %s or placeholder", got, mctokenFileName)
	}
}

// TestWriteDiagnostic_Golden pins the rendered output shape of
// writeDiagnostic. The "Details (paste into bug reports):" block plus
// the indentation and section spacing are user-visible and frequently
// what shows up in bug reports verbatim. NO_COLOR is forced via the
// package's existing init knob so the snapshot is stable across TTYs.
func TestWriteDiagnostic_Golden(t *testing.T) {
	// Save and restore the color state via the existing knobs.
	savedRed, savedYellow, savedReset := colorRed, colorYellow, colorReset
	colorRed, colorYellow, colorReset = "", "", ""
	t.Cleanup(func() { colorRed, colorYellow, colorReset = savedRed, savedYellow, savedReset })

	d := diagnostic{
		headline: "Couldn't reach Xbox Live",
		body:     "device.auth.xboxlive.com isn't responding from your network.",
		causes:   []string{"ISP block", "Corporate firewall"},
		fix:      "Try: curl -v https://device.auth.xboxlive.com/\nIf curl also hangs, use a VPN.",
	}
	raw := errors.New("mint mctoken: ... dial tcp ...: connectex: ...")
	var buf strings.Builder
	writeDiagnostic(&buf, d, raw)

	want := "\n" +
		"  Error: Couldn't reach Xbox Live\n" +
		"\n" +
		"    device.auth.xboxlive.com isn't responding from your network.\n" +
		"\n" +
		"    Common causes:\n" +
		"      - ISP block\n" +
		"      - Corporate firewall\n" +
		"\n" +
		"    Try: curl -v https://device.auth.xboxlive.com/\n" +
		"    If curl also hangs, use a VPN.\n" +
		"\n" +
		"  Details (paste into bug reports):\n" +
		"    mint mctoken: ... dial tcp ...: connectex: ...\n" +
		"\n"
	if got := buf.String(); got != want {
		t.Errorf("writeDiagnostic output mismatch.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestWriteDiagnostic_MinimalDiagnostic verifies the body / causes /
// fix sections collapse correctly when the diagnostic only has a
// headline. This is the "I have a vague match but no actionable
// detail" shape.
func TestWriteDiagnostic_MinimalDiagnostic(t *testing.T) {
	savedRed, savedYellow, savedReset := colorRed, colorYellow, colorReset
	colorRed, colorYellow, colorReset = "", "", ""
	t.Cleanup(func() { colorRed, colorYellow, colorReset = savedRed, savedYellow, savedReset })

	raw := errors.New("raw err")
	var buf strings.Builder
	writeDiagnostic(&buf, diagnostic{headline: "Something happened"}, raw)
	got := buf.String()
	// Headline and Details: are always present; the body / causes / fix
	// blocks should NOT appear at all (no stray blank section headers).
	if !strings.Contains(got, "Error: Something happened") {
		t.Errorf("missing headline:\n%s", got)
	}
	if !strings.Contains(got, "Details (paste into bug reports):") {
		t.Errorf("missing Details block:\n%s", got)
	}
	if strings.Contains(got, "Common causes:") {
		t.Errorf("Common causes header appeared with empty causes:\n%s", got)
	}
}

// TestWriteIndented_TrimsTrailingNewline verifies the P2.1 fix: a body
// that ends in \n should NOT render a phantom blank line. Was a
// footgun on diagnostics built via fmt.Sprintf that closed with %v\n.
func TestWriteIndented_TrimsTrailingNewline(t *testing.T) {
	var buf strings.Builder
	writeIndented(&buf, "one\ntwo\n", "  ")
	got := buf.String()
	want := "  one\n  two\n"
	if got != want {
		t.Errorf("writeIndented with trailing \\n = %q, want %q", got, want)
	}
}

// TestTimeoutDiag_CurlHintConditional verifies the P0.1 fix: when the
// timeout host isn't a Microsoft endpoint, we don't suggest a curl
// against xboxlive (which would be misleading on, say, a CDN timeout).
func TestTimeoutDiag_CurlHintConditional(t *testing.T) {
	t.Run("microsoft host keeps curl hint", func(t *testing.T) {
		err := errors.New(`Post "https://device.auth.xboxlive.com/x": dial tcp: i/o timeout`)
		d := timeoutDiag(err)
		if !strings.Contains(d.fix, "curl -v https://device.auth.xboxlive.com") {
			t.Errorf("microsoft host should keep curl hint, got fix:\n%s", d.fix)
		}
	})
	t.Run("third-party host suppresses curl hint", func(t *testing.T) {
		err := errors.New(`Post "https://cdn.example.com/pack": dial tcp: i/o timeout`)
		d := timeoutDiag(err)
		if strings.Contains(d.fix, "xboxlive.com") {
			t.Errorf("third-party host should not mention xboxlive, got fix:\n%s", d.fix)
		}
		if strings.Contains(d.fix, "Quick check") {
			t.Errorf("third-party host should not include the Quick check curl line, got fix:\n%s", d.fix)
		}
	})
	t.Run("empty host suppresses curl hint", func(t *testing.T) {
		err := errors.New(`some bare timeout with no URL: i/o timeout`)
		d := timeoutDiag(err)
		if strings.Contains(d.fix, "xboxlive.com") {
			t.Errorf("empty host should not mention xboxlive, got fix:\n%s", d.fix)
		}
	})
}
