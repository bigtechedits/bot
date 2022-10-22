package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/r3labs/sse/v2"
)

// handleRecentChanges starts a Go routine that receives events
// from the Wikipedia recent changes stream.
func handleRecentChanges(ctx context.Context, events chan *wikiEvent) error {
	go func() {
		var lastID string
		var c *sse.Client
		bufferPow := 18

		// Every 15 minutes Wikipedia terminates the connection. So use a loop to
		// reconnect and get the events.
		for {
			c = sse.NewClient("https://stream.wikimedia.org/v2/stream/recentchange",
				sse.ClientMaxBufferSize(1<<bufferPow))
			c.EventID = lastID
			if err := c.SubscribeWithContext(ctx, "", func(msg *sse.Event) {
				if len(msg.Data) == 0 {
					return
				}
				var ev wikiEvent
				lastID = string(msg.ID)
				err := json.Unmarshal(msg.Data, &ev)
				if err != nil {
					log.Printf("Failed to unmarshal data: %v\n%v\n", err, msg.Data)
					return
				}
				if ev.Bot {
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
				// Only log unexpected errors. NO_ERROR is expected every 15 minutes
				// because of the connection termination by wikipedia.
				if !strings.Contains(err.Error(), "NO_ERROR") {
					if strings.Contains(err.Error(), "token too long") {
						// dynamically increase the scanner buffer size of the client.
						bufferPow++
						if bufferPow >= 24 {
							// memory isn't free. so apply a limit
							os.Exit(1)
						}
					} else {
						log.Printf("Failed to subscribe: %v", err)
					}
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
