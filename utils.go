package main

import (
	"log"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/timechainlabs/torrent"
	"github.com/timechainlabs/torrent/bencode"
)

const (
	MICRO_PAY_EXTENSION_ID uint32 = 10
)

var builtinAnnounceList = [][]string{
	{"http://p4p.arenabg.com:1337/announce"},
	{"udp://tracker.opentrackr.org:1337/announce"},
	{"udp://tracker.openbittorrent.com:6969/announce"},
}

func createAndSendExtendedMessageWithTransaction(userWallet *FullWallet, peerConnection *torrent.PeerConn,
	r torrent.Request, outputAmount uint64) bool {
	var microTransaction = transaction.NewTransaction()

	microTransaction.AddOutput(&transaction.TransactionOutput{
		Satoshis:      outputAmount,
		LockingScript: userWallet.LockingScript,
	})

	bencodeBytes, err := bencode.Marshal(&MicroPayRequest{
		Type:  "REQUEST",
		Txhex: microTransaction.Hex(),
	})
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return false
	}

	err = peerConnection.WriteExtendedMessage(MICRO_PAY_EXTENSION_ID, bencodeBytes)
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return false
	}

	return false
}
