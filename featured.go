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

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/franchise"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
	"github.com/sandertv/go-raknet"
	"golang.org/x/oauth2"
)

const (
	featuredPingConcurrency = 5
	featuredPingTimeout     = 3 * time.Second
	// 60s covers Discover + cold PlayFab mint (10-15s alone) + gatherings POST.
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
			return errors.New(lang.T("featured.download.needindex"))
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
	fmt.Println(lang.T("featured.usage"))
}

func featuredList() error {
	fmt.Println(lang.T("featured.list.header.title"))
	fmt.Println(lang.T("featured.list.header.source"))
	fmt.Println(lang.T("featured.list.header.rule"))

	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()

	// Auth prints must precede the spinner - their fmt.Println would
	// otherwise splice into the spinner's redrawn line.
	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	fmt.Println()
	sp := startSpinner(lang.T("featured.list.spinner.fetch"))
	servers, _, err := fetchFeaturedListWithClient(sigCtx, tokenSource)
	sp.stop("")
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		fmt.Println(lang.T("featured.list.empty"))
		return nil
	}

	sp = startSpinner(lang.Tf("featured.list.spinner.ping", len(servers)))
	pingAll(sigCtx, servers)
	sp.stop("")

	printFeaturedTable(servers)
	fmt.Println(lang.T("featured.list.todownload"))
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
		return fmt.Errorf(lang.T("featured.download.badindex"), args[0])
	}

	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	fmt.Println()
	sp := startSpinner(lang.T("featured.download.spinner.fetch"))
	servers, client, err := fetchFeaturedListWithClient(sigCtx, tokenSource)
	sp.stop("")
	if err != nil {
		return err
	}
	if idx > len(servers) {
		return fmt.Errorf(lang.T("featured.download.outofrange"), idx, len(servers))
	}
	s := servers[idx-1]

	resolveCtx, cancel := context.WithTimeout(sigCtx, featuredAPITimeout)
	defer cancel()

	address, err := resolveAddress(resolveCtx, client, s)
	if err != nil {
		return err
	}

	fmt.Printf(lang.T("featured.download.resolved"), s.Name, address)

	return runDownload(append([]string{address}, args[1:]...))
}

// resolveAddress turns a Server into a host:port. Partner-direct
// entries return inline; experience/gathering rows go through the
// franchise API.
func resolveAddress(ctx context.Context, client *franchise.Client, s franchise.Server) (string, error) {
	switch s.Kind {
	case franchise.KindPartnerDirect:
		return s.Address(), nil
	case franchise.KindGathering:
		host, port, err := resolveWithRetry(client, func() (string, int, error) {
			return client.Venue(ctx, s.GatheringID)
		})
		if err != nil {
			if errors.Is(err, franchise.ErrForbidden) {
				return "", fmt.Errorf(lang.T("featured.resolve.gathering.forbidden"), s.Name)
			}
			return "", fmt.Errorf(lang.T("featured.resolve.gathering.fail"), s.Name, err)
		}
		return fmt.Sprintf("%s:%d", host, port), nil
	case franchise.KindPartnerExperience:
		host, port, err := resolveWithRetry(client, func() (string, int, error) {
			return client.JoinExperience(ctx, s.ExperienceID)
		})
		if err != nil {
			switch {
			case errors.Is(err, franchise.ErrExperienceOffline):
				return "", fmt.Errorf(lang.T("featured.resolve.experience.offline"), s.Name)
			case errors.Is(err, franchise.ErrForbidden):
				return "", fmt.Errorf(lang.T("featured.resolve.experience.forbidden"), s.Name)
			}
			return "", fmt.Errorf(lang.T("featured.resolve.experience.fail"), s.Name, err)
		}
		return fmt.Sprintf("%s:%d", host, port), nil
	}
	return "", fmt.Errorf(lang.T("featured.resolve.unknownkind"), s.Name)
}

// resolveWithRetry re-mints the MCToken once on ErrAuthRejected (401),
// mirroring the catalog fetch - a server-revoked but time-valid cached token
// would otherwise strand the resolve. It deliberately does NOT retry on
// ErrForbidden (403): a fresh token can't grant access the account lacks.
func resolveWithRetry(client *franchise.Client, call func() (string, int, error)) (string, int, error) {
	host, port, err := call()
	if errors.Is(err, franchise.ErrAuthRejected) {
		invalidateFranchise(client)
		host, port, err = call()
	}
	return host, port, err
}

// fetchFeaturedListWithClient pulls the partner catalog and live-events
// list, merging gatherings to the top to match the in-game Servers tab.
// Returns the client so resolveAddress can reuse its cached MCToken.
// One retry on ErrAuthRejected handles a server-revoked but time-valid
// cached MCToken.
func fetchFeaturedListWithClient(parent context.Context, tokenSource oauth2.TokenSource) ([]franchise.Server, *franchise.Client, error) {
	ctx, cancel := context.WithTimeout(parent, featuredAPITimeout)
	defer cancel()

	client, err := newFranchiseClient(ctx, tokenSource)
	if err != nil {
		return nil, nil, err
	}

	for attempt := range 2 {
		partners, err := client.PartnerCatalog(ctx)
		if err != nil {
			if errors.Is(err, franchise.ErrAuthRejected) && attempt == 0 {
				invalidateFranchise(client)
				continue
			}
			return nil, nil, err
		}
		// Non-auth gatherings failures don't abort - partner rows are
		// still useful and most cohorts have empty events most of the time.
		var gatherings []franchise.Server
		if g, gErr := client.LiveEvents(ctx); gErr == nil {
			gatherings = g
		} else if errors.Is(gErr, franchise.ErrAuthRejected) && attempt == 0 {
			invalidateFranchise(client)
			continue
		} else {
			fmt.Fprintf(os.Stderr, lang.T("featured.liveevents.warn"), gErr)
		}
		persistFranchiseToken(client)
		return append(gatherings, partners...), client, nil
	}
	return nil, nil, franchise.ErrAuthRejected
}

// pingAll fills Online/Players/MOTD on every entry with a public
// address. Semaphore is acquired before goroutine spawn to bound
// goroutine count and let cancel hit queued work too.
func pingAll(ctx context.Context, servers []franchise.Server) {
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

func pingOne(parent context.Context, s *franchise.Server) {
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

func printFeaturedTable(servers []franchise.Server) {
	const (
		nameColMin = 4
		addrColMin = 7
	)

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

// addressColumn renders inline host:port once a row has an address (resolved
// or direct), or a resolve-on-download placeholder for experience/gathering
// rows that haven't been resolved yet.
func addressColumn(s franchise.Server) string {
	if s.HasAddress() {
		return s.Address()
	}
	switch s.Kind {
	case franchise.KindGathering:
		return lang.T("featured.addr.liveevent")
	case franchise.KindPartnerExperience:
		return lang.T("featured.addr.experience")
	}
	return lang.T("featured.addr.none")
}

func tagFor(s franchise.Server) (tag, color string) {
	if !s.HasAddress() {
		switch s.Kind {
		case franchise.KindGathering:
			return "[EVT]", colorYellow
		case franchise.KindPartnerExperience:
			return "[EXP]", colorCyan
		}
		return "[OFF]", colorRed
	}
	if !s.Online {
		return "[OFF]", colorRed
	}
	return "[ON]", colorGreen
}

func statusFor(s franchise.Server) string {
	if !s.HasAddress() {
		if s.Kind == franchise.KindGathering || s.Kind == franchise.KindPartnerExperience {
			return lang.T("featured.status.resolve")
		}
		return lang.T("featured.status.offline")
	}
	if !s.Online {
		return lang.T("featured.status.offline")
	}
	if s.Players <= 0 {
		return lang.T("featured.status.online")
	}
	if s.Players == 1 {
		return lang.T("featured.status.online.one")
	}
	return lang.Tf("featured.status.online.many", humanCount(s.Players))
}

// coloredStatusFor wraps statusFor in the same state color the CLI tag column
// uses (online green / offline red / live-event yellow / experience cyan), so
// the interactive featured list reads at a glance like the recent and decrypt
// screens instead of being monochrome.
func coloredStatusFor(s franchise.Server) string {
	_, color := tagFor(s)
	return color + statusFor(s) + colorReset
}

// humanCount: 14104 -> "14k", 1_543_000 -> "1.5M", <1000 unchanged.
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
