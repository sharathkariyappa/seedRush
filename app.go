package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"seedrush/broadcaster"
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/bsv-blockchain/go-sdk/wallet"
	"github.com/bsv-blockchain/go-sdk/wallet/substrates"
	"github.com/timechainlabs/torrent"
	"github.com/timechainlabs/torrent/bencode"
	"github.com/timechainlabs/torrent/metainfo"
	pp "github.com/timechainlabs/torrent/peer_protocol"
	"github.com/timechainlabs/torrent/storage"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func NewApp() *App {
	return &App{
		broadcaster:    broadcaster.NewBroadcaster(),
		torrents:       make(map[string]*torrent.Torrent),
		torrentsState:  make(map[string]*TorrentFundState),
		downloadSpeeds: make(map[string]*speedTracker),
		uploadSpeeds:   make(map[string]*speedTracker),
		pausedTorrents: make(map[string]bool),
	}
}

func (a *App) startup(ctx context.Context) {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	a.ctx = ctx

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	a.downloadDir = filepath.Join(homeDir, "seedrush", "downloads")
	a.stateFile = filepath.Join(homeDir, "seedrush", "torrents.json")
	a.piecesDir = filepath.Join(homeDir, "seedrush", "pieces")

	err = os.MkdirAll(a.downloadDir, 0755)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	err = os.MkdirAll(a.piecesDir, 0755)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	_, err = os.Stat(filepath.Join(homeDir, "seedrush", "wif.txt"))
	if err != nil {
		a.wallet, err = createWallet()
		if err != nil {
			log.Default().Fatalf("Error: %s\n", err.Error())
		}

		file, err := os.OpenFile(
			filepath.Join(homeDir, "seedrush", "wif.txt"), os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0755,
		)
		if err != nil {
			log.Default().Fatalf("Error: %s\n", err.Error())
		}

		defer file.Close()

		_, err = file.Write([]byte(a.wallet.PrivateKey.Wif()))
		if err != nil {
			log.Default().Fatalf("Error: %s\n", err.Error())
		}
	} else {
		a.wallet, err = loadWallet(filepath.Join(homeDir, "seedrush", "wif.txt"))
		if err != nil {
			log.Default().Fatalf("Error: %s\n", err.Error())
		}
	}

	err = a.wallet.Sync(true)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	log.Default().Printf("Address: %s\n", a.wallet.WalletAddress.AddressString)

	bitClientConfig := torrent.NewDefaultClientConfig()
	bitClientConfig.DataDir = a.downloadDir
	bitClientConfig.Seed = true
	bitClientConfig.Callbacks = torrent.Callbacks{
		PeerConnReadExtensionMessage: []func(torrent.PeerConnReadExtensionMessageEvent){
			func(m torrent.PeerConnReadExtensionMessageEvent) {
				if m.ExtensionNumber == pp.ExtensionNumber(MICRO_PAY_EXTENSION_ID) {
					var microPayRequest MicroPayRequest
					extensionError := bencode.Unmarshal(m.Payload, &microPayRequest)
					if extensionError != nil {
						log.Default().Printf("Error: %s\n", extensionError.Error())
						return
					}

					switch microPayRequest.Type {

					case "SENT":
						{
							extensionError = a.broadcaster.Broadcast(microPayRequest.Txhex)
							if extensionError != nil {
								log.Default().Printf("Error: %s\n", extensionError.Error())
								return
							} else {
								if m.PeerConn == nil {
									return
								} else {
									var t *torrent.Torrent = m.PeerConn.Torrent()
									if t != nil {
										state, found := a.torrentsState[t.InfoHash().String()]
										if found {
											state.SatoshisEarned += 100
										}
									}

									m.PeerConn.ReleaseRequest()
								}
							}
						}

					case "REQUEST":
						{
							a.walletLocker.Lock()
							defer a.walletLocker.Unlock()

							var microTransaction *transaction.Transaction
							microTransaction, extensionError = transaction.NewTransactionFromHex(microPayRequest.Txhex)
							if extensionError != nil {
								log.Default().Printf("Error: %s\n", extensionError.Error())
								return
							}

							var totalInputAmount uint64
							for i := range a.wallet.WalletUtxos {
								extensionError = microTransaction.AddInputFrom(
									a.wallet.WalletUtxos[i].Txid,
									a.wallet.WalletUtxos[i].Vout,
									a.wallet.LockingScript.String(),
									a.wallet.WalletUtxos[i].Satoshis,
									a.wallet.UnlockingScriptTemplate,
								)
								if extensionError != nil {
									log.Default().Printf("Error: %s\n", extensionError.Error())
									return
								}

								totalInputAmount += a.wallet.WalletUtxos[i].Satoshis
							}

							extensionError = microTransaction.Sign()
							if extensionError != nil {
								log.Default().Printf("Error: %s\n", extensionError.Error())
								return
							}

							var totalSpentAmount uint64 = microTransaction.TotalOutputSatoshis() + (20 * uint64(len(microTransaction.Inputs))) + 10
							if totalInputAmount < totalSpentAmount {
								log.Default().Printf("Insufficient funds\n")
								return
							}

							if totalInputAmount > totalSpentAmount {
								microTransaction.AddOutput(&transaction.TransactionOutput{
									Satoshis:      totalInputAmount - totalSpentAmount,
									LockingScript: a.wallet.LockingScript,
								})

								a.wallet.WalletUtxos = []UTXO{
									{1, (totalInputAmount - totalSpentAmount), microTransaction.TxID().String()},
								}

								wailsruntime.EventsEmit(a.ctx, "wallet-updated")
							}

							log.Default().Printf("Txid: %s\n", microTransaction.TxID().String())

							microPayRequest.Type = "SENT"
							microPayRequest.Txhex = microTransaction.Hex()
							payload, extensionError := bencode.Marshal(&microPayRequest)
							if extensionError != nil {
								log.Default().Printf("Error: %s\n", extensionError.Error())
								return
							}

							extensionError = m.PeerConn.WriteExtendedMessage(MICRO_PAY_EXTENSION_ID, payload)
							if extensionError != nil {
								log.Default().Printf("Error: %s\n", extensionError.Error())
								return
							}

							var t *torrent.Torrent = m.PeerConn.Torrent()
							if t != nil {
								state, found := a.torrentsState[t.InfoHash().String()]
								if found {
									state.SatoshisSpend += 100
								}
							}
						}

					default:
						return
					}
				}
			},
		},
		ApproveOrNotPieceRequest: func(p *torrent.PeerConn, r torrent.Request) bool {
			return createAndSendExtendedMessageWithTransaction(a.wallet, p, r, 100)
		},
	}

	a.client, err = torrent.NewClient(bitClientConfig)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	a.loadSavedTorrents()

	go func() {
		var timer = time.Tick(10 * time.Second)

		for range timer {
			a.speedStatsLocker.Lock()
			a.updateStatsLoop()
			a.speedStatsLocker.Unlock()
		}
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	a.saveTorrentsState()

	if a.client != nil {
		a.client.Close()
	}
}

func (a *App) AddMagnet(magnetURI string) error {
	if a.client == nil {
		return fmt.Errorf("torrent client not initialized")
	}

	t, err := a.client.AddMagnet(magnetURI)
	if err != nil {
		return fmt.Errorf("failed to add magnet: %w", err)
	}

	go func() {
		<-t.GotInfo()

		a.appStateLocker.Lock()
		defer a.appStateLocker.Unlock()

		a.speedStatsLocker.Lock()
		defer a.speedStatsLocker.Unlock()

		var infoHash string = t.InfoHash().String()

		a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
		a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}

		a.torrents[infoHash] = t

		a.saveTorrentsState()

		t.AllowDataDownload()
		t.AllowDataUpload()
		t.DownloadAll()
		t.Seeding()

		wailsruntime.EventsEmit(a.ctx, "torrents-updated")
	}()

	return nil
}

func (a *App) CreateTorrentFromPath(path string) (*string, error) {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	a.speedStatsLocker.Lock()
	defer a.speedStatsLocker.Unlock()

	if a.client == nil {
		return nil, fmt.Errorf("torrent client not initialized")
	}

	var metaInfo metainfo.MetaInfo = metainfo.MetaInfo{
		AnnounceList: builtinAnnounceList,
		CreationDate: time.Now().Unix(),
	}

	var info = metainfo.Info{
		PieceLength: 64 * DATA_UNIT,
	}

	err := info.BuildFromFilePath(path)
	if err != nil {
		return nil, err
	}

	metaInfo.InfoBytes, err = bencode.Marshal(&info)
	if err != nil {
		return nil, err
	}

	pieceInformationStorage, err := storage.NewDefaultPieceCompletionForDir(a.piecesDir)
	if err != nil {
		return nil, err
	}

	t, _ := a.client.AddTorrentOpt(torrent.AddTorrentOpts{
		InfoBytes: metaInfo.InfoBytes,
		InfoHash:  metaInfo.HashInfoBytes(),
		Storage: storage.NewFileOpts(storage.NewFileClientOpts{
			ClientBaseDir: path,
			FilePathMaker: func(opts storage.FilePathMakerOpts) string {
				return filepath.Join(opts.File.Path...)
			},
			PieceCompletion: pieceInformationStorage,
		}),
	})

	err = t.MergeSpec(&torrent.TorrentSpec{
		Trackers: [][]string{{
			"wss://tracker.btorrent.xyz",
			"wss://tracker.openwebtorrent.com",
			"http://p4p.arenabg.com:1337/announce",
			"udp://tracker.opentrackr.org:1337/announce",
			"udp://tracker.openbittorrent.com:6969/announce",
		}},
	})
	if err != nil {
		return nil, err
	}

	<-t.GotInfo()
	t.AllowDataDownload()
	t.AllowDataUpload()
	t.DownloadAll()
	t.Seeding()

	var infoHash string = t.InfoHash().String()

	a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
	a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}

	a.torrents[infoHash] = t

	a.saveTorrentsState()

	wailsruntime.EventsEmit(a.ctx, "torrents-updated")

	magnetLink, err := metaInfo.MagnetV2()
	if err != nil {
		return nil, err
	}

	var magnetUrl string = magnetLink.String()

	return &magnetUrl, nil
}

func (a *App) GetTorrents() ([]*SeedRushTorrentInfo, error) {
	a.appStateLocker.RLock()
	defer a.appStateLocker.RUnlock()

	a.speedStatsLocker.RLock()
	defer a.speedStatsLocker.RUnlock()

	var torrents []*SeedRushTorrentInfo
	for hash := range a.torrents {
		info, err := a.getTorrentInfo(hash)
		if err != nil {
			log.Default().Printf("Error: %s\n", err.Error())
			return nil, err
		}

		torrents = append(torrents, info)
	}

	return torrents, nil
}

func (a *App) GetTorrent(infoHash string) (*SeedRushTorrentInfo, error) {
	a.appStateLocker.RLock()
	defer a.appStateLocker.RUnlock()

	return a.getTorrentInfo(infoHash)
}

func (a *App) PauseTorrent(infoHash string) error {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	t, exists := a.torrents[infoHash]
	if !exists {
		return fmt.Errorf("torrent not found")
	}

	t.DisallowDataDownload()
	t.DisallowDataUpload()
	a.pausedTorrents[infoHash] = true
	a.saveTorrentsState()

	return nil
}

func (a *App) ResumeTorrent(infoHash string) error {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	t, exists := a.torrents[infoHash]
	if !exists {
		return fmt.Errorf("torrent not found")
	}

	delete(a.pausedTorrents, infoHash)

	go func() {
		<-t.GotInfo()

		a.appStateLocker.Lock()
		defer a.appStateLocker.Unlock()

		a.speedStatsLocker.Lock()
		defer a.speedStatsLocker.Unlock()

		var infoHash string = t.InfoHash().String()

		a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
		a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}

		a.torrents[infoHash] = t

		t.AllowDataDownload()
		t.AllowDataUpload()
		t.DownloadAll()
		t.Seeding()

		a.saveTorrentsState()

		wailsruntime.EventsEmit(a.ctx, "torrents-updated")
	}()

	return nil
}

func (a *App) RemoveTorrent(infoHash string, deleteFiles bool) error {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	a.speedStatsLocker.Lock()
	defer a.speedStatsLocker.Unlock()

	t, exists := a.torrents[infoHash]
	if !exists {
		return fmt.Errorf("torrent not found")
	}

	t.DisallowDataDownload()
	t.DisallowDataUpload()

	delete(a.torrents, infoHash)
	delete(a.pausedTorrents, infoHash)
	delete(a.downloadSpeeds, infoHash)
	delete(a.uploadSpeeds, infoHash)
	delete(a.torrentsState, infoHash)

	if deleteFiles && t.Info() != nil {
		for _, file := range t.Files() {
			os.Remove(filepath.Join(a.downloadDir, file.Path()))
		}
	}

	t.Drop()
	a.saveTorrentsState()

	return nil
}

func (a *App) GetStats() *Stats {
	a.appStateLocker.RLock()
	defer a.appStateLocker.RUnlock()

	a.speedStatsLocker.RLock()
	defer a.speedStatsLocker.RUnlock()

	return a.getStats()
}

func (a *App) OpenDownloadFolder() error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "explorer"
		args = []string{a.downloadDir}
	case "darwin":
		cmd = "open"
		args = []string{a.downloadDir}
	default:
		cmd = "xdg-open"
		args = []string{a.downloadDir}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		return fmt.Errorf("failed to open download folder: %w", err)
	}

	return nil
}

func (a *App) SelectSeedPath() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:                      "Select Directory/Packages To Seed",
		TreatPackagesAsDirectories: true,
	})
}

func (a *App) GetWalletState() *WalletState {
	a.walletLocker.RLock()
	defer a.walletLocker.RUnlock()

	var balance uint64
	for i := range a.wallet.WalletUtxos {
		balance += a.wallet.WalletUtxos[i].Satoshis
	}

	return &WalletState{
		WalletBalance: balance,
		WalletAddress: a.wallet.WalletAddress.AddressString,
	}
}

func (a *App) WalletSync() error {
	a.walletLocker.Lock()
	defer a.walletLocker.Unlock()

	return a.wallet.Sync(true)
}

func (a *App) RequestFunds(amount uint64) error {
	a.walletLocker.Lock()
	defer a.walletLocker.Unlock()

	var brc100Wallet = substrates.NewHTTPWalletJSON("https://seedrush.online", "http://localhost:3321", nil)
	if brc100Wallet == nil {
		return errors.New("bsv wallet not found")
	}

	actionResult, err := brc100Wallet.CreateAction(context.Background(), wallet.CreateActionArgs{
		Description: "TopUp Seedrush Wallet",
		Outputs: []wallet.CreateActionOutput{
			wallet.CreateActionOutput{
				OutputDescription: "TopUp Seedrush Wallet Output",
				Satoshis:          amount,
				LockingScript:     a.wallet.LockingScript.Bytes(),
				Tags:              []string{"SEEDRUSH"},
			},
		},
		Labels: []string{"SEEDRUSH"},
	})
	if err != nil {
		return err
	}

	a.wallet.WalletUtxos = append(a.wallet.WalletUtxos, UTXO{
		Vout:     0,
		Satoshis: amount,
		Txid:     actionResult.Txid.String(),
	})

	return nil
}

func (a *App) saveTorrentsState() {
	var states []TorrentState
	for hash, t := range a.torrents {
		var magnetURI string
		if t.Info() != nil {
			var metaInfo = t.Metainfo()
			magnetLink, err := metaInfo.MagnetV2()
			if err != nil {
				continue
			}

			magnetURI = magnetLink.String()
		}

		existingState, found := a.torrentsState[hash]
		if found {
			states = append(states, TorrentState{
				IsPaused:       a.pausedTorrents[hash],
				SatoshisEarned: existingState.SatoshisEarned,
				SatoshisSpend:  existingState.SatoshisSpend,
				InfoHash:       hash,
				MagnetURI:      magnetURI,
				UpdatedAt:      time.Now(),
			})
		} else {
			a.torrentsState[hash] = new(TorrentFundState)
			states = append(states, TorrentState{
				IsPaused:  a.pausedTorrents[hash],
				InfoHash:  hash,
				MagnetURI: magnetURI,
				UpdatedAt: time.Now(),
			})
		}
	}

	data, err := json.Marshal(&states)
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return
	}

	err = os.WriteFile(a.stateFile, data, 0644)
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return
	}
}

func (a *App) loadSavedTorrents() {
	data, err := os.ReadFile(a.stateFile)
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return
	}

	var states []TorrentState
	err = json.Unmarshal(data, &states)
	if err != nil {
		log.Default().Printf("Error: %s\n", err.Error())
		return
	}

	for i := range states {
		t, err := a.client.AddMagnet(states[i].MagnetURI)
		if err != nil {
			continue
		}

		var infoHash = t.InfoHash().String()

		a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
		a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}

		a.torrents[infoHash] = t
		a.torrentsState[infoHash] = &TorrentFundState{
			SatoshisEarned: states[i].SatoshisEarned,
			SatoshisSpend:  states[i].SatoshisSpend,
		}

		if states[i].IsPaused {
			a.pausedTorrents[infoHash] = true
		} else {
			go func() {
				<-t.GotInfo()
				t.AllowDataDownload()
				t.AllowDataUpload()
				t.Seeding()
			}()
		}
	}
}

func (a *App) getTorrentInfo(hash string) (*SeedRushTorrentInfo, error) {
	t, found := a.torrents[hash]
	if !found {
		return nil, errors.New("torrent not found")
	}

	var isPaused bool = a.pausedTorrents[hash]
	var stats torrent.TorrentStats = t.Stats()

	var progress float64
	if t.Length() > 0 {
		progress = float64(t.BytesCompleted()) / float64(t.Length()) * 100
	}

	var files []SeedRushFileInfo
	if t.Info() != nil {
		for _, file := range t.Files() {
			var fileProgress float64
			if file.Length() > 0 {
				fileProgress = float64(file.BytesCompleted()) / float64(file.Length()) * 100
			}

			files = append(files, SeedRushFileInfo{
				Size:     file.Length(),
				Progress: fileProgress,
				Name:     file.DisplayPath(),
				SizeStr:  formatBytes(file.Length()),
				Path:     file.Path(),
			})
		}
	}

	var downloadSpeed, uploadSpeed int64
	if tracker, ok := a.downloadSpeeds[hash]; ok {
		downloadSpeed = tracker.speed
	}

	if tracker, ok := a.uploadSpeeds[hash]; ok {
		uploadSpeed = tracker.speed
	}

	var eta string = "Unknown"
	if downloadSpeed > 0 && t.BytesCompleted() < t.Length() {
		eta = formatDuration(time.Duration(((t.Length() - t.BytesCompleted()) / downloadSpeed)))
	}

	return &SeedRushTorrentInfo{
		IsPaused:       isPaused,
		Peers:          stats.ActivePeers,
		Seeds:          stats.ConnectedSeeders,
		DownloadSpeed:  downloadSpeed,
		UploadSpeed:    uploadSpeed,
		Size:           t.Length(),
		SatoshisSpend:  a.torrentsState[hash].SatoshisSpend,
		SatoshisEarned: a.torrentsState[hash].SatoshisEarned,
		Progress:       progress,
		ID:             hash,
		Name:           t.Name(),
		InfoHash:       hash,
		SizeStr:        formatBytes(t.Length()),
		Status:         a.getTorrentStatus(t, stats, isPaused),
		DownloadedStr:  formatSpeed(downloadSpeed),
		UploadedStr:    formatSpeed(uploadSpeed),
		ETA:            eta,
		Files:          files,
		UpdatedAt:      time.Now(),
	}, nil
}

func (a *App) getTorrentStatus(t *torrent.Torrent, stats torrent.TorrentStats, isPaused bool) string {
	if isPaused {
		return "paused"
	}

	if t.Info() == nil || t.Length() == 0 {
		return "loading"
	}

	if t.BytesCompleted() >= t.Length() {
		if stats.ActivePeers > 0 {
			return "seeding"
		}

		return "completed"
	}

	if stats.ActivePeers > 0 {
		return "downloading"
	}

	return "stalled"
}

func (a *App) getStats() *Stats {
	var totalDown, totalUp int64
	var activeTorrents, activePeers int

	for hash := range a.torrents {
		if tracker, ok := a.downloadSpeeds[hash]; ok {
			totalDown += tracker.speed
		}

		if tracker, ok := a.uploadSpeeds[hash]; ok {
			totalUp += tracker.speed
		}
	}

	for hash, t := range a.torrents {
		if !a.pausedTorrents[hash] && t.BytesCompleted() < t.Length() {
			activeTorrents++
		}

		activePeers += t.Stats().ActivePeers
	}

	return &Stats{
		TotalDownloadSpeed: formatSpeed(totalDown),
		TotalUploadSpeed:   formatSpeed(totalUp),
		ActiveTorrents:     activeTorrents,
		TotalPeers:         activePeers,
	}
}

func (a *App) updateStatsLoop() {
	for hash, t := range a.torrents {
		var stats torrent.TorrentStats = t.Stats()
		var now time.Time = time.Now()

		if tracker, ok := a.downloadSpeeds[hash]; ok {
			elapsed := now.Sub(tracker.lastTime).Seconds()
			if elapsed > 0 {
				currentBytes := stats.BytesReadData.Int64()
				tracker.speed = int64(float64(currentBytes-tracker.lastBytes) / elapsed)
				tracker.lastBytes = currentBytes
				tracker.lastTime = now
			}
		}

		if tracker, ok := a.uploadSpeeds[hash]; ok {
			elapsed := now.Sub(tracker.lastTime).Seconds()
			if elapsed > 0 {
				currentBytes := stats.BytesWrittenData.Int64()
				tracker.speed = int64(float64(currentBytes-tracker.lastBytes) / elapsed)
				tracker.lastBytes = currentBytes
				tracker.lastTime = now
			}
		}
	}

	a.lastUpdateTime = time.Now()
	wailsruntime.EventsEmit(a.ctx, "torrents-updated")
}

func formatBytes(bytes int64) string {
	if bytes < DATA_UNIT {
		return fmt.Sprintf("%d B", bytes)
	}

	var div, exp int64 = int64(DATA_UNIT), 0
	for n := bytes / DATA_UNIT; n >= DATA_UNIT; n /= DATA_UNIT {
		div *= DATA_UNIT
		exp++
	}

	return fmt.Sprintf("%.1f %cB", (float64(bytes) / float64(div)), "KMGTPE"[exp])
}

func formatSpeed(bytesPerSec int64) string {
	return formatBytes(bytesPerSec) + "/s"
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	return fmt.Sprintf("%dh %dm", int(d.Hours()), (int(d.Minutes()) % 60))
}
