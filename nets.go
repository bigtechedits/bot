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
	mappings *ip2asn.Map
	// The majority of access will be read access. But once every while we need to update the
	// map
	mu sync.RWMutex
}

func init() {
	// List of ASNs used by big tech cloud providers.
	asn2provider = make(map[int]string)
	providers := map[string][]int{
		"Google": {
			15169,
			19527,
			36040,
			36385,
			43515,
			45566,
			16550,
			139070,
			139190,
			394089,
			396982,
		},
		"Microsoft": {
			8075,
			8068,
			8069,
		},
		"Amazon": {
			8987,
			14618,
			16509,
			36263,
			399834,
		},
		"Cloudflare": {
			13335,
		},
		"Akamai": {
			12222,
			16625,
		},
		"DigitalOcean": {
			14061,
			46652,
			62567,
			133165,
			135340,
			200130,
			201229,
			202018,
			202109,
			393406,
			394362,
		},
		"Oracle": {
			90,
			7160,
			15179,
			31898,
			33517,
		},
		"Apple": {
			714,
			6185,
		},
		"Meta": {
			32934,
		},
		"Hetzner": {
			24940,
			212317,
			213230,
			215859,
		},
		"Tencent": {
			45090,
			132203,
			132591,
		},
		"Alibaba": {
			24429,
			37963,
			45102,
		},
		"OVH": {
			16276,
			35540,
		},
		"Yandex": {
			13238,
			200350,
			208722,
		},
		"Baidu": {
			38627,
			55967,
		},
		"Fastly": {
			54113,
		},
		"IBM": {
			10337,
			24189,
		},
	}

	for provider, asns := range providers {
		for _, asn := range asns {
			asn2provider[asn] = provider
		}
	}
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
