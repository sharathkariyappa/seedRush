package broadcaster

import (
	"net/http"
	"time"
)

type IBroadcaster interface {
	Broadcast(string) error
}

func NewBroadcaster() IBroadcaster {

	return &broadcaster{
		config: &ARCConfig{
			TaalURL:   "https://arc.taal.com/v1",
			TaalToken: "mainnet_0ded86c89f5dd923d93f56cb615d43d2",
			WaitFor:   "ACCEPTED_BY_NETWORK",
		},
		httpClient: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: false,
				ForceAttemptHTTP2: false,
				IdleConnTimeout:   (10 * time.Minute),
			},
		},
	}
}
