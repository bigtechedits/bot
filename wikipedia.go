package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/r3labs/sse/v2"
)

// handleRecentChanges starts a Go routine that receives events
// from the Wikipedia recent changes stream.
func handleRecentChanges(ctx context.Context, events chan *wikiEvent) error {
	go func() {
		var lastID []byte
		var c *sse.Client
		bufferPow := 18

		// Every 15 minutes Wikipedia terminates the connection. So use a loop to
		// reconnect and get the events.
		for {
			c = sse.NewClient("https://stream.wikimedia.org/v2/stream/recentchange",
				sse.ClientMaxBufferSize(1<<bufferPow))
			c.LastEventID.Store(lastID)

			c.Headers = map[string]string{
				"User-Agent": "bigtechedits/bot",
			}

			c.Connection.Transport = &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				TLSHandshakeTimeout: 10 * time.Second,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 10 * time.Second,
				}).DialContext,
			}

			c.ReconnectNotify = func(err error, next time.Duration) {
				log.Println("Reconnecting to stream.wikimedia.org/v2/stream/recentchange after ", next, "due to", err)
			}

			c.OnConnect(func(c *sse.Client) {
				log.Println("connected to stream.wikimedia.org/v2/stream/recentchange")
			})
			c.OnDisconnect(func(c *sse.Client) {
				log.Println("disconnect from stream.wikimedia.org/v2/stream/recentchange")
			})

			if err := c.SubscribeWithContext(ctx, "", func(msg *sse.Event) {
				if len(msg.Data) == 0 {
					return
				}
				var ev wikiEvent
				lastID = msg.ID
				err := json.Unmarshal(msg.Data, &ev)
				if err != nil {
					log.Printf("Failed to unmarshal data: %v\n%v\n", err, msg.Data)
					return
				}
				if ev.Bot {
					return
				}

				if ev.Meta.Domain == "canary" {
					// Ignore canary events.
					return
				}

				// Filter out some changes we do not want to tweet about.
				if strings.HasPrefix(ev.Title, "User talk:") ||
					strings.HasPrefix(ev.Title, "Talk:") ||
					strings.HasPrefix(ev.Title, "Diskussion:") ||
					strings.HasPrefix(ev.Title, "Wikipedia:Tutorial") ||
					strings.HasPrefix(ev.Title, "Category:") ||
					strings.HasPrefix(ev.Title, "File:") ||
					strings.HasPrefix(ev.Title, "Template:") {
					return
				}
				if ev.Revision.New == 0 || ev.Revision.Old == 0 {
					return
				}
				events <- &ev
			}); err != nil {
				switch {
				case errors.Is(err, io.ErrUnexpectedEOF):
					// ErrUnexpectedEOF is expected when the connection is terminated by Wikipedia
					// every ~15 minutes.
				case strings.Contains(err.Error(), "token too long"):
					// dynamically increase the scanner buffer size of the client.
					bufferPow++
					if bufferPow >= 24 {
						// memory isn't free. so apply a limit
						os.Exit(1)
					}
				default:
					// Only log unexpected errors.
					log.Printf("Failed to subscribe: %v", err)
					time.Sleep(3 * time.Second)
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	return nil
}
