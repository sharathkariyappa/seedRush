package broadcaster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

func (b *broadcaster) Broadcast(hex string) error {

	var retryCount uint8
	var err error

TAAL:
	if retryCount > 50 {
		return err
	}

	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		time.Sleep(time.Second)
	}

	taalRequest, err := http.NewRequest(
		http.MethodPost,
		(b.config.TaalURL + "/tx"),
		bytes.NewBuffer(fmt.Appendf(nil, "{\"rawTx\":\"%s\"}", hex)),
	)
	if err != nil {
		retryCount++
		goto TAAL
	}

	taalRequest.Header.Set("Content-Type", "application/json")
	taalRequest.Header.Set("X-CumulativeFeeValidation", "true")
	taalRequest.Header.Set("Authorization", b.config.TaalToken)
	taalRequest.Header.Set("X-WaitFor", b.config.WaitFor)
	taalRequest.Header.Set("X-ForceValidation", "false")
	taalRequest.Header.Set("X-SkipScriptValidation", "true")
	taalRequest.Header.Set("X-SkipTxValidation", "true")

	taalResponse, err := b.httpClient.Do(taalRequest)
	if err != nil {
		retryCount++
		goto TAAL
	}

	defer taalResponse.Body.Close()

	if taalResponse.StatusCode > 299 {

		var errorDetails map[string]any = make(map[string]any)

		err = json.NewDecoder(taalResponse.Body).Decode(&errorDetails)
		if err != nil {
			retryCount++
			goto TAAL
		}

		errorMessage, ok := errorDetails["detail"].(string)
		if !ok {
			err = fmt.Errorf("response code received from TAAL ARC: %d", taalResponse.StatusCode)
		} else {
			err = errors.New(errorMessage)
		}

		retryCount++
		goto TAAL
	}

	return nil
}
