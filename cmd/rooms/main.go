// Command rooms is Multi-Enso, the game's online rooms, as a program of its own: rooms live in
// its memory, so it is restarted only when its own code changes, and releasing the site leaves
// the rooms playing. It holds no secrets and reads no database: it asks the site
// (internal/multiplay/roomsapi).
//
//	ourtaiko-rooms                 serves the rooms
//	ourtaiko-rooms drain -wait 20m lets the rounds under way end, begins no others, and returns
//	                               when none is left or the wait is over
//
// Configured by the environment:
//
//	MULTIPLAY_SITE_INTERNAL_URL  where the site answers the rooms (its MULTIPLAY_INTERNAL_ADDR)
//	MULTIPLAY_LISTEN_ADDR        where the gRPC service listens
//	MULTIPLAY_ADMIN_ADDR         health and draining, for this container only
//	MULTIPLAY_MIN_PLAYERS        fewest players a round needs; default 2
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ourtaiko.dev/net/internal/multiplay"
	"ourtaiko.dev/net/internal/multiplay/roomsapi"
)

const defaultAdminAddr = "127.0.0.1:8083"

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "drain" {
		if err := drain(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := serve(); err != nil {
		log.Fatal(err)
	}
}

func serve() error {
	site := os.Getenv("MULTIPLAY_SITE_INTERNAL_URL")
	if site == "" {
		return errors.New("MULTIPLAY_SITE_INTERNAL_URL must say where the site answers the rooms")
	}
	minPlayers := 0
	if raw := os.Getenv("MULTIPLAY_MIN_PLAYERS"); raw != "" {
		var err error
		if minPlayers, err = strconv.Atoi(raw); err != nil || minPlayers < 1 {
			return errors.New("MULTIPLAY_MIN_PLAYERS must be a number from 1 up")
		}
	}
	timings, err := multiplay.TimingsWith(minPlayers)
	if err != nil {
		return err
	}
	client := roomsapi.NewClient(site)
	rooms := multiplay.NewServer(multiplay.ServerConfig{Catalog: client, Authenticate: client.Authenticate, Timings: timings})

	adminAddr := env("MULTIPLAY_ADMIN_ADDR", defaultAdminAddr)
	listener, err := net.Listen("tcp", adminAddr)
	if err != nil {
		return fmt.Errorf("the admin address %s: %w", adminAddr, err)
	}
	admin := &http.Server{Handler: rooms.AdminHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := admin.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("the admin address stopped: %v", err)
		}
	}()

	listen := env("MULTIPLAY_LISTEN_ADDR", "127.0.0.1:50061")
	if err = rooms.Start(listen); err != nil {
		return fmt.Errorf("Multi-Enso cannot listen on %s: %w", listen, err)
	}
	log.Printf("Multi-Enso (gRPC, h2c) listening on %s; asking the site at %s", rooms.Addr(), site)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	rooms.Stop()
	admin.Close()
	return nil
}

// drain asks the running server, in this container, to let its rounds end.
func drain(args []string) error {
	flags := flag.NewFlagSet("drain", flag.ExitOnError)
	wait := flags.Duration("wait", 20*time.Minute, "how long to wait for the rounds under way")
	flags.Parse(args)
	base := "http://" + env("MULTIPLAY_ADMIN_ADDR", defaultAdminAddr) + "/drain"

	// The server takes rounds again by itself if nobody replaces it soon after the wait.
	limit := *wait + 5*time.Minute
	state, err := askDrain(http.MethodPost, base+"?for="+limit.String())
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*wait)
	var lastSaid time.Time
	for ; ; time.Sleep(2 * time.Second) {
		if state.Busy == 0 {
			fmt.Printf("no round is under way (%d rooms open)\n", state.Rooms)
			return nil
		}
		if !time.Now().Before(deadline) {
			fmt.Printf("gave up waiting after %s: %d rounds are still under way\n", *wait, state.Busy)
			return nil
		}
		if time.Since(lastSaid) >= 30*time.Second {
			fmt.Printf("waiting for %d rounds to end\n", state.Busy)
			lastSaid = time.Now()
		}
		if state, err = askDrain(http.MethodGet, base); err != nil {
			return err
		}
	}
}

func askDrain(method, url string) (multiplay.DrainState, error) {
	var state multiplay.DrainState
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return state, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return state, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return state, fmt.Errorf("the server answered %s", response.Status)
	}
	return state, json.NewDecoder(response.Body).Decode(&state)
}
