package broadcaster

import (
	"net/http"
)

type ARCConfig struct {
	TaalURL   string
	TaalToken string
	WaitFor   string
}

type broadcaster struct {
	config     *ARCConfig
	httpClient *http.Client
}
