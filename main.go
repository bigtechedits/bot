package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/oauth2"
)

type meta struct {
	URI    string `json:"uri"`
	Domain string `json:"domain"`
}

type revision struct {
	Old uint64 `json:"old"`
	New uint64 `json:"new"`
}

// wikiEvent is a subset of the /mediawiki/recentchange/1.0.0 schema.
type wikiEvent struct {
	Meta     meta     `json:"meta"`
	Title    string   `json:"title"`
	User     string   `json:"user"`
	Bot      bool     `json:"bot"`
	Revision revision `json:"revision"`
}

type eventEntry struct {
	addr     netip.Addr
	ev       *wikiEvent
	provider string
	oldID    uint64
	newID    uint64
	ts       time.Time
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := populateASNMap(); err != nil {
		log.Printf("Failed to update IP to ASN map: %v", err)
		return
	}

	client, token, err := connect(ctx)
	if err != nil {
		log.Printf("Failed to connect to Twitter: %v", err)
		return
	}

	events := make(chan *wikiEvent)

	go handleEvent(ctx, client, token, events)

	go func() {
		ticker := time.NewTicker(26 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := populateASNMap(); err != nil {
					log.Printf("Failed to update IP to ASN map: %v", err)
				}
			}
		}
	}()

	if err := handleRecentChanges(ctx, events); err != nil {
		log.Printf("Failed to listen to Wikipedia recent changes event stream: %v", err)
		return
	}

	// Block until context is stoped.
	<-ctx.Done()
}

func handleEvent(ctx context.Context, client *http.Client, token *oauth2.Token,
	events chan *wikiEvent) {
	eventStore := make(map[string]eventEntry)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("Dropping %d changes", len(eventStore))
			return
		case ev := <-events:
			addr, err := netip.ParseAddr(ev.User)
			if err != nil {
				continue
			}
			provider, bigTechOrigin := isBigTechOrigin(addr)
			if !bigTechOrigin {
				continue
			}

			entry, exist := eventStore[ev.Meta.URI]
			if !exist {
				eventStore[ev.Meta.URI] = eventEntry{
					addr:     addr,
					ev:       ev,
					provider: provider,
					oldID:    ev.Revision.Old,
					newID:    ev.Revision.New,
					ts:       time.Now(),
				}
				continue
			}
			if entry.addr.Compare(addr) == 0 {
				// The same IP did further changes.
				entry.ts = time.Now()
				entry.newID = ev.Revision.New
				eventStore[ev.Meta.URI] = entry
				continue
			}
			// A different source made changes to the same Wiki data
			if err := tweetChange(ctx, client, token, entry); err != nil {
				log.Printf("Failed to tweet %#v: %v", entry, err)
			}
			// Remove the just tweeted entry
			delete(eventStore, ev.Meta.URI)

			// Don't tweet immediately to avoid becoming a spamer.
			eventStore[ev.Meta.URI] = eventEntry{
				addr:     addr,
				ev:       ev,
				provider: provider,
				oldID:    ev.Revision.Old,
				newID:    ev.Revision.New,
				ts:       time.Now(),
			}
		case <-ticker.C:
			for _, entry := range eventStore {
				if time.Since(entry.ts) > 23*time.Minute {
					if err := tweetChange(ctx, client, token, entry); err != nil {
						log.Printf("Failed to tweet %#v: %v", entry, err)
					}
					delete(eventStore, entry.ev.Meta.URI)
				}
			}
		}
	}
}

type displaytitle struct {
	Parse struct {
		Title        string `json:"title"`
		Pageid       int    `json:"pageid"`
		Displaytitle string `json:"displaytitle"`
	} `json:"parse"`
}

func getWikidataDisplayTitle(origTitle string) (string, error) {
	resp, err := http.Get("https://www.wikidata.org/w/api.php?page=" + origTitle +
		"&action=parse&prop=displaytitle&format=json")
	if err != nil {
		return origTitle, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return origTitle, err
	}

	var dt displaytitle
	if err := json.Unmarshal(data, &dt); err != nil {
		return origTitle, err
	}
	doc, err := html.Parse(strings.NewReader(dt.Parse.Displaytitle))
	if err != nil {
		return origTitle, err
	}
	var newTitle string
	var found bool
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == 1 {
			tmpTitle := strings.TrimSpace(n.Data)
			if len(tmpTitle) != 0 && !strings.Contains(tmpTitle, origTitle) {
				newTitle = tmpTitle
				found = true
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	if found {
		return newTitle, nil
	}
	return origTitle, nil
}
