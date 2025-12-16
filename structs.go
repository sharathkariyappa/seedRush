package main

import (
	"context"
	"seedrush/broadcaster"
	"sync"
	"time"

	primitives "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/timechainlabs/torrent"
)

const DATA_UNIT = 1024

type speedTracker struct {
	lastBytes int64
	speed     int64
	lastTime  time.Time
}

type SeedRushTorrentInfo struct {
	IsPaused       bool               `json:"isPaused"`
	Peers          int                `json:"peers"`
	Seeds          int                `json:"seeds"`
	Size           int64              `json:"size"`
	DownloadSpeed  int64              `json:"downloadSpeed"`
	UploadSpeed    int64              `json:"uploadSpeed"`
	SatoshisSpend  uint64             `json:"satoshisSpend"`
	SatoshisEarned uint64             `json:"satoshisEarned"`
	Progress       float64            `json:"progress"`
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	InfoHash       string             `json:"infoHash"`
	SizeStr        string             `json:"sizeStr"`
	Status         string             `json:"status"`
	DownloadedStr  string             `json:"downloadSpeedStr"`
	UploadedStr    string             `json:"uploadSpeedStr"`
	ETA            string             `json:"eta"`
	UpdatedAt      time.Time          `json:"addedAt"`
	Files          []SeedRushFileInfo `json:"files"`
}

type SeedRushFileInfo struct {
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Name     string  `json:"name"`
	SizeStr  string  `json:"sizeStr"`
	Path     string  `json:"path"`
}

type Stats struct {
	TotalDownloadSpeed string `json:"totalDownload"`
	TotalUploadSpeed   string `json:"totalUpload"`
	ActiveTorrents     int    `json:"activeTorrents"`
	TotalPeers         int    `json:"totalPeers"`
}

type TorrentState struct {
	IsPaused       bool      `json:"isPaused"`
	SatoshisSpend  uint64    `json:"satoshisSpend"`
	SatoshisEarned uint64    `json:"satoshisEarned"`
	InfoHash       string    `json:"infoHash"`
	MagnetURI      string    `json:"magnetUri,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type TorrentFundState struct {
	SatoshisSpend  uint64 `json:"satoshisSpend"`
	SatoshisEarned uint64 `json:"satoshisEarned"`
}

type App struct {
	piecesDir        string
	downloadDir      string
	stateFile        string
	ctx              context.Context
	broadcaster      broadcaster.IBroadcaster
	client           *torrent.Client
	wallet           *FullWallet
	appStateLocker   sync.RWMutex
	speedStatsLocker sync.RWMutex
	walletLocker     sync.RWMutex
	lastUpdateTime   time.Time
	pausedTorrents   map[string]bool
	torrents         map[string]*torrent.Torrent
	torrentsState    map[string]*TorrentFundState
	downloadSpeeds   map[string]*speedTracker
	uploadSpeeds     map[string]*speedTracker
}

type MicroPayRequest struct {
	Type  string `bencode:"type"`
	Txhex string `bencode:"txhex"`
}

type UTXO struct {
	Vout     uint32 `json:"vout"`
	Satoshis uint64 `json:"satoshis"`
	Txid     string `json:"txid"`
}

type UtxosResponse struct {
	Utxos []UTXO `json:"unspent"`
}

type FullWallet struct {
	WalletAddress           *script.Address
	LockingScript           *script.Script
	PrivateKey              *primitives.PrivateKey
	UnlockingScriptTemplate transaction.UnlockingScriptTemplate
	LastSync                time.Time
	WalletUtxos             []UTXO
}

type WalletState struct {
	WalletBalance uint64 `json:"balance"`
	WalletAddress string `json:"address"`
}
