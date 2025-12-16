package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	bip32 "github.com/bsv-blockchain/go-sdk/compat/bip32"
	"github.com/bsv-blockchain/go-sdk/compat/bip39"
	primitives "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	transaction "github.com/bsv-blockchain/go-sdk/transaction/chaincfg"
	sighash "github.com/bsv-blockchain/go-sdk/transaction/sighash"
	"github.com/bsv-blockchain/go-sdk/transaction/template/p2pkh"
)

func createWallet() (*FullWallet, error) {
	seed, err := bip39.NewEntropy(256)
	if err != nil {
		return nil, err
	}

	mnemonic, err := bip39.NewMnemonic(seed)
	if err != nil {
		return nil, err
	}

	masterKey, err := bip32.NewMaster(
		bip39.NewSeed(mnemonic, rand.Text()),
		&transaction.MainNet,
	)
	if err != nil {
		return nil, err
	}

	privateKey, err := masterKey.ECPrivKey()
	if err != nil {
		return nil, err
	}

	address, err := script.NewAddressFromPublicKey(privateKey.PubKey(), true)
	if err != nil {
		return nil, err
	}

	lockingScript, err := p2pkh.Lock(address)
	if err != nil {
		return nil, err
	}

	sighashFlags := sighash.ForkID | sighash.None | sighash.AnyOneCanPay
	unlockingScriptTemplate, err := p2pkh.Unlock(privateKey, &sighashFlags)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	return &FullWallet{
		LockingScript:           lockingScript,
		PrivateKey:              privateKey,
		WalletAddress:           address,
		LastSync:                time.Now(),
		UnlockingScriptTemplate: unlockingScriptTemplate,
	}, nil
}

func loadWallet(path string) (*FullWallet, error) {
	wifBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	privateKey, err := primitives.PrivateKeyFromWif(string(wifBytes))
	if err != nil {
		return nil, err
	}

	address, err := script.NewAddressFromPublicKey(privateKey.PubKey(), true)
	if err != nil {
		return nil, err
	}

	lockingScript, err := p2pkh.Lock(address)
	if err != nil {
		return nil, err
	}

	sighashFlags := sighash.ForkID | sighash.None | sighash.AnyOneCanPay
	unlockingScriptTemplate, err := p2pkh.Unlock(privateKey, &sighashFlags)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	return &FullWallet{
		LockingScript:           lockingScript,
		PrivateKey:              privateKey,
		WalletAddress:           address,
		LastSync:                time.Now(),
		UnlockingScriptTemplate: unlockingScriptTemplate,
	}, nil
}

func (w *FullWallet) Sync() error {
	if time.Since(w.LastSync) < (5 * time.Minute) {
		return nil
	}

	utxosRequest, err := http.NewRequest(
		http.MethodGet,
		"https://api.bitails.io/address/"+w.WalletAddress.AddressString+"/unspent?limit=1000",
		nil,
	)
	if err != nil {
		return err
	}

	utxosRequest.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(utxosRequest)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	var utxosResponse UtxosResponse
	err = json.NewDecoder(response.Body).Decode(&utxosResponse)
	if err != nil {
		return err
	}

	w.WalletUtxos = utxosResponse.Utxos

	return nil
}
