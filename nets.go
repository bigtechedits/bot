package main

import (
	"io"
	"net/http"
	"net/netip"
	"os"
	"sync"

	"github.com/bradfitz/ip2asn"
)

var (
	// List of ASN to Big Tech providers.
	asn2provider map[int]string

	m ipMappings
)

type ipMappings struct {
	// The majority of access will be read access. But once every while we need to update the
	// map.
	mu       sync.RWMutex
	mappings *ip2asn.Map
}

func init() {
	// List of ASNs used by big tech cloud providers.
	asn2provider = make(map[int]string)
	asn2provider[132203] = "Tencent"
	asn2provider[13238] = "Yandex"
	asn2provider[132591] = "Tencent"
	asn2provider[139070] = "Google"
	asn2provider[139190] = "Google"
	asn2provider[14061] = "DigitalOcean"
	asn2provider[14618] = "Amazon"
	asn2provider[15169] = "Google"
	asn2provider[16509] = "Amazon"
	asn2provider[16550] = "Google"
	asn2provider[19527] = "Google"
	asn2provider[208722] = "Yandex"
	asn2provider[24429] = "Alibaba"
	asn2provider[31898] = "Oracle"
	asn2provider[36040] = "Google"
	asn2provider[36385] = "Google"
	asn2provider[37963] = "Alibaba"
	asn2provider[38627] = "Baidu"
	asn2provider[43515] = "Google"
	asn2provider[45090] = "Tencent"
	asn2provider[45102] = "Alibaba"
	asn2provider[45566] = "Google"
	asn2provider[55967] = "Baidu"
	asn2provider[8075] = "Microsoft"
}

func downloadIPToASN() (string, error) {
	file, err := os.CreateTemp("", "latest-asn")
	if err != nil {
		return "", err
	}
	defer file.Close()
	resp, err := http.Get("https://iptoasn.com/data/ip2asn-combined.tsv.gz")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}
	return file.Name(), nil
}

func populateASNMap() error {
	f, err := downloadIPToASN()
	if err != nil {
		return err
	}
	defer os.RemoveAll(f)
	tmp, err := ip2asn.OpenFile(f)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings = tmp
	return nil
}

// isBigTechOrigin returns true if addr is from a known Big Tech ASN
func isBigTechOrigin(addr netip.Addr) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	asn := m.mappings.ASofIP(addr)
	if bigTech, ok := asn2provider[asn]; ok {
		return bigTech, true
	}
	return "", false
}
