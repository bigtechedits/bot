package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	authCodeURLFile = "/opt/bigtechedits/AuthCodeURL"
	authCodeFile    = "/opt/bigtechedits/AuthCode"
)

// Set the Go build internal vcs.revision as User-Agent suffix to simplify debugging.
func setUserAgent(req *http.Request) {
	var appendix string
	appendix = fmt.Sprintf("%d", time.Now().Unix())

	buildinfo, ok := debug.ReadBuildInfo()
	if ok {
		for _, v := range buildinfo.Settings {
			if strings.Compare(v.Key, "vcs.revision") == 0 {
				appendix = v.Value
				break
			}
		}
	}
	req.Header.Set("User-Agent", fmt.Sprintf("BigTechEditsBot-%s", appendix))
}

// Connect establishes an authorized session to Twitter.
func connect(ctx context.Context) (*http.Client, *oauth2.Token, error) {
	clientID := os.Getenv("TWITTER_CLIENT_ID")
	clientSecret := os.Getenv("TWITTER_CLIENT_SECRET")
	redirectURI := os.Getenv("TWITTER_REDIRECT_URI")

	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return nil, nil, fmt.Errorf("missing environment variable")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"tweet.write", "offline.access", "tweet.read", "users.read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://twitter.com/i/oauth2/authorize",
			TokenURL: "https://api.twitter.com/2/oauth2/token",
		},
	}
	a := oauth2.SetAuthURLParam("code_challenge", "challenge")
	b := oauth2.SetAuthURLParam("code_challenge_method", "plain")
	c := oauth2.SetAuthURLParam("redirect_uri", redirectURI)

	// Make sure files do not exist from previous runs
	if err := os.Remove(authCodeURLFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if err := os.Remove(authCodeFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}

	url := conf.AuthCodeURL(fmt.Sprintf("%d", time.Now().Unix()), a, b, c)
	log.Printf("Visit the URL for the auth dialog:\n%v\n", url)
	if err := os.WriteFile(authCodeURLFile, []byte(url), 0o666); err != nil {
		return nil, nil, err
	}

	var code string
	authTimeout, authCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer authCancel()
	for {
		select {
		case <-authTimeout.Done():
			return nil, nil, fmt.Errorf("authentiation timed out")
		default:
			content, err := os.ReadFile(authCodeFile)
			switch {
			case errors.Is(err, os.ErrNotExist):
				// Avoid a busy loop reading the file with the auth code
				time.Sleep(10 * time.Second)
				continue
			case err == nil:
				code = string(content)
				goto breakout
			default:
				return nil, nil, err
			}
		}
	}
breakout:
	d := oauth2.SetAuthURLParam("code_verifier", "challenge")
	e := oauth2.SetAuthURLParam("grant_type", "authorization_code")
	f := oauth2.SetAuthURLParam("client_id", clientID)
	tok, err := conf.Exchange(ctx, code, c, d, e, f)
	if err != nil {
		return nil, nil, err
	}
	client := conf.Client(ctx, tok)
	return client, tok, nil
}

// Test if the given URL exists.
func verifyDiffURL(url string) error {
	_, err := http.Get(url)
	return err
}

// tweetChange posts a tweet about a wikipedia recent change event.
func tweetChange(ctx context.Context, client *http.Client, token *oauth2.Token,
	entry eventEntry) error {
	uri, err := url.ParseRequestURI(entry.ev.Meta.URI)
	if err != nil {
		return err
	}

	log.Printf("%#v", entry.ev)

	diffURL := fmt.Sprintf("https://%s/w/index.php?title=%s&diff=%d&oldid=%d",
		entry.ev.Meta.Domain,
		url.PathEscape(uri.Path),
		entry.newID,
		entry.oldID)

	if err := verifyDiffURL(diffURL); err != nil {
		log.Printf("bogus diff url: %s\t%#v\n", diffURL, entry)
		return err
	}

	var title string
	wikitype := "#wikipedia"
	if strings.Contains(entry.ev.Meta.Domain, "wikidata.org") {
		wikitype = "#wikidata"
		var err error
		title, err = getWikidataDisplayTitle(entry.ev.Title)
		if err != nil {
			log.Printf("failed to get wikidata title: %v", err)
			// Use the original title and continue
			title = entry.ev.Title
		}
	} else {
		title = entry.ev.Title
	}
	if len(title) > 80 {
		log.Printf("shortening title: %v\n", entry.ev)
		title = fmt.Sprintf("%s..", entry.ev.Title[:76])
	}

	tweet := fmt.Sprintf("{\"text\": \"%s entry \\\"%s\\\" edited anonymously from #%s %s\"}",
		wikitype,
		title,
		entry.provider,
		diffURL)

	body := strings.NewReader(tweet)

	req, err := http.NewRequest("POST", "https://api.twitter.com/2/tweets", body)
	if err != nil {
		return err
	}
	setUserAgent(req)
	req.Header.Set("Authorization",
		fmt.Sprintf("%s %s", token.TokenType, token.AccessToken))
	req.Header.Set("Content-type", "application/json")
	r, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusCreated {
		failure, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read body from failed tweet: %v", err)
		}
		log.Printf("Response of failed (code %d) failure: %s\ttweet: %s", r.StatusCode,
			string(failure), tweet)
		return fmt.Errorf("tweet failed with error code %d", r.StatusCode)
	}
	return nil
}
