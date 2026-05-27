package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sandertv/go-raknet"
	"golang.org/x/oauth2"
)

const (
	featuredPingConcurrency = 5
	featuredPingTimeout     = 3 * time.Second
	// featuredAPITimeout has to cover Discover + a cold PlayFab mint +
	// the gatherings POST. A fresh PlayFab login alone can use 10-15s
	// when the on-disk MCToken cache is empty; 60s keeps the slow path
	// comfortable without burying real network failures.
	featuredAPITimeout = 60 * time.Second
)

func runFeatured(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}

	switch sub {
	case "list":
		return featuredList()
	case "download":
		if len(args) < 2 {
			return fmt.Errorf("featured download requires an index - run 'bedrock-pack-tools featured' to see the list")
		}
		return featuredDownload(args[1:])
	case "-h", "--help", "help":
		printFeaturedUsage()
		return nil
	default:
		// Bare index like "featured 1" is shorthand for "featured download 1".
		if _, err := strconv.Atoi(sub); err == nil {
			return featuredDownload(args)
		}
		printFeaturedUsage()
		return errUsage
	}
}

func printFeaturedUsage() {
	fmt.Println(`Usage:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download <index> [output-dir]

List the Featured Servers and Live Events surfaced by Minecraft's
client-discovery service, then optionally download a chosen entry.

Requires Xbox Live authentication (token cached on first use).

Examples:
  bedrock-pack-tools featured
  bedrock-pack-tools featured download 1
  bedrock-pack-tools featured download 1 ./packs/`)
}

func featuredList() error {
	fmt.Println("\n  ┌─ Featured Servers ────────────────────────")
	fmt.Println("  │ Source: Minecraft gatherings API")
	fmt.Println("  └──────────────────────────────────────────")

	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()

	// Auth output must happen BEFORE the spinner starts, otherwise its
	// fmt.Println splices into the spinner's redrawn line and leaves a
	// frozen "spinner + Auth: ..." string on screen.
	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	fmt.Println()
	sp := startSpinner("Fetching catalog")
	servers, _, err := fetchFeaturedListWithClient(sigCtx, tokenSource)
	sp.stop("")
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		fmt.Println("  No featured servers returned by the API.")
		return nil
	}

	sp = startSpinner(fmt.Sprintf("Pinging %d servers", len(servers)))
	pingAll(sigCtx, servers)
	sp.stop("")

	printFeaturedTable(servers)
	fmt.Println("\n  To download:  bedrock-pack-tools featured download <index> [output-dir]")
	fmt.Println()
	return nil
}

func featuredDownload(args []string) error {
	if len(args) < 1 {
		printFeaturedUsage()
		return errUsage
	}
	idx, err := strconv.Atoi(args[0])
	if err != nil || idx < 1 {
		return fmt.Errorf("invalid index %q: must be a positive integer", args[0])
	}

	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	fmt.Println()
	sp := startSpinner("Fetching catalog")
	servers, client, err := fetchFeaturedListWithClient(sigCtx, tokenSource)
	sp.stop("")
	if err != nil {
		return err
	}
	if idx > len(servers) {
		return fmt.Errorf("index %d out of range (have %d featured servers)", idx, len(servers))
	}
	s := servers[idx-1]

	resolveCtx, cancel := context.WithTimeout(sigCtx, featuredAPITimeout)
	defer cancel()

	address, err := resolveAddress(resolveCtx, client, s)
	if err != nil {
		return err
	}

	fmt.Printf("\n  [->] %s  ->  %s\n", s.Name, address)

	downloadArgs := []string{address}
	downloadArgs = append(downloadArgs, args[1:]...)
	return runDownload(downloadArgs)
}

// resolveAddress turns a [featuredServer] into a connectable host:port.
// Partner-direct entries return their inline address; experience and
// gathering entries trigger the appropriate franchise-service call to
// pick up the current venue, with helpful errors when nothing is live.
// The client is the one already constructed by fetchFeaturedList, so we
// reuse the cached MCToken instead of running auth twice.
func resolveAddress(ctx context.Context, client *gatheringsClient, s featuredServer) (string, error) {
	switch s.Kind {
	case kindPartnerDirect:
		return s.Address(), nil
	case kindGathering, kindPartnerExperience:
		tok, err := client.Token(ctx)
		if err != nil {
			return "", err
		}
		if s.Kind == kindGathering {
			host, port, err := venueAddress(ctx, client.gatheringsURI, tok.AuthorizationHeader, s.GatheringID)
			if err != nil {
				return "", fmt.Errorf("resolve gathering %q: %w", s.Name, err)
			}
			return fmt.Sprintf("%s:%d", host, port), nil
		}
		host, port, err := joinExperience(ctx, client.gatheringsURI, tok.AuthorizationHeader, s.ExperienceID)
		if err != nil {
			if errors.Is(err, errExperienceOffline) {
				return "", fmt.Errorf("%q has no active venue right now (the slot is listed but not joinable from outside the official client)", s.Name)
			}
			return "", fmt.Errorf("resolve experience %q: %w", s.Name, err)
		}
		return fmt.Sprintf("%s:%d", host, port), nil
	}
	return "", fmt.Errorf("unknown featured kind for %q", s.Name)
}

// fetchFeaturedListWithClient authenticates and pulls both the partner
// catalog (POST /discovery/blob/client) and the live-events list
// (GET /config/public), merging gatherings to the top - that's the
// order the in-game Servers tab uses. Returns the [gatheringsClient]
// too so a follow-up [resolveAddress] call can reuse the cached MCToken
// instead of re-authenticating. Retries once after a server-side token
// rejection so a server-revoked but time-valid cached MCToken doesn't
// strand the user.
func fetchFeaturedListWithClient(parent context.Context, tokenSource oauth2.TokenSource) ([]featuredServer, *gatheringsClient, error) {
	ctx, cancel := context.WithTimeout(parent, featuredAPITimeout)
	defer cancel()

	client, err := newGatheringsClient(ctx, tokenSource)
	if err != nil {
		return nil, nil, err
	}

	for attempt := range 2 {
		tok, err := client.Token(ctx)
		if err != nil {
			return nil, nil, err
		}
		partners, err := fetchPartnerCatalog(ctx, client.gatheringsURI, tok.AuthorizationHeader)
		if err != nil {
			if errors.Is(err, errAuthRejected) && attempt == 0 {
				client.invalidate()
				continue
			}
			return nil, nil, err
		}
		// Gathering fetch failures (other than auth) don't have to abort
		// the whole list — an empty events slot is the normal case for
		// most cohorts most of the time, and the partner rows are still
		// useful on their own.
		var gatherings []featuredServer
		if g, gErr := fetchGatherings(ctx, client.gatheringsURI, tok.AuthorizationHeader); gErr == nil {
			gatherings = g
		} else if errors.Is(gErr, errAuthRejected) && attempt == 0 {
			client.invalidate()
			continue
		} else {
			fmt.Fprintf(os.Stderr, "  Warning: could not fetch live events: %v\n", gErr)
		}
		return append(gatherings, partners...), client, nil
	}
	return nil, nil, errAuthRejected
}

// pingAll fills in Online/Players/MOTD on every entry with a public
// address. The semaphore is acquired *before* the goroutine spawn so we
// genuinely bound goroutine count, not just in-flight work, and so a
// cancel cancels everything still queued. ctx propagates to each ping.
func pingAll(ctx context.Context, servers []featuredServer) {
	sem := make(chan struct{}, featuredPingConcurrency)
	var wg sync.WaitGroup
	for i := range servers {
		if !servers[i].HasAddress() {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			pingOne(ctx, &servers[i])
		}(i)
	}
	wg.Wait()
}

func pingOne(parent context.Context, s *featuredServer) {
	ctx, cancel := context.WithTimeout(parent, featuredPingTimeout)
	defer cancel()
	data, err := raknet.PingContext(ctx, s.Address())
	if err != nil {
		return
	}
	s.Online = true
	// RakNet ping returns ";"-delimited fields: MCPE;motd;protocol;
	// version;players;maxPlayers;guid;subMotd;gamemode;...
	parts := strings.Split(string(data), ";")
	if len(parts) > 4 {
		if n, err := strconv.Atoi(parts[4]); err == nil {
			s.Players = n
		}
	}
	if len(parts) > 1 {
		s.MOTD = parts[1]
	}
}

func printFeaturedTable(servers []featuredServer) {
	const (
		nameColMin = 4
		addrColMin = 7
	)

	// Compute column widths from actual data so no cell overflows its lane.
	idxWidth := len(strconv.Itoa(len(servers)))
	nameWidth := nameColMin
	addrWidth := addrColMin
	for _, s := range servers {
		if w := len(s.Name); w > nameWidth {
			nameWidth = w
		}
		addr := addressColumn(s)
		if w := len(addr); w > addrWidth {
			addrWidth = w
		}
	}

	rowFmt := fmt.Sprintf("  %%s  [%%%dd]  %%-%ds  %%-%ds  %%s\n", idxWidth, nameWidth, addrWidth)

	fmt.Println()
	for i, s := range servers {
		tag, color := tagFor(s)
		tagCol := fmt.Sprintf("%s%-5s%s", color, tag, colorReset)
		fmt.Printf(rowFmt, tagCol, i+1, s.Name, addressColumn(s), statusFor(s))
	}
}

// addressColumn renders the second-to-last column of the table. Direct
// partners show their inline host:port; experience and gathering rows
// show a tag indicating which API call would resolve them on download.
func addressColumn(s featuredServer) string {
	switch s.Kind {
	case kindGathering:
		return "(live event)"
	case kindPartnerExperience:
		return "(experience-join)"
	}
	if s.HasAddress() {
		return s.Address()
	}
	return "(no address)"
}

func tagFor(s featuredServer) (tag, color string) {
	switch s.Kind {
	case kindGathering:
		return "[EVT]", colorYellow
	case kindPartnerExperience:
		return "[EXP]", colorCyan
	}
	switch {
	case !s.HasAddress(), !s.Online:
		return "[OFF]", colorRed
	default:
		return "[ON]", colorGreen
	}
}

func statusFor(s featuredServer) string {
	switch s.Kind {
	case kindGathering:
		return "resolve on download"
	case kindPartnerExperience:
		return "resolve on download"
	}
	if !s.Online {
		return "offline"
	}
	if s.Players <= 0 {
		return "online"
	}
	return fmt.Sprintf("online %s players", humanCount(s.Players))
}

// humanCount renders integer counts in a compact form: 14104 -> "14k",
// 1_543_000 -> "1.5M", anything under 1000 stays as-is.
func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return strconv.Itoa(n)
	}
}
