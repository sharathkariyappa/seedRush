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
	"time"

	"github.com/bsv-blockchain/go-sdk/transaction"
	"github.com/timechainlabs/torrent"
	"github.com/timechainlabs/torrent/bencode"
	"github.com/timechainlabs/torrent/metainfo"
	pp "github.com/timechainlabs/torrent/peer_protocol"
	"github.com/timechainlabs/torrent/storage"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func NewApp() *App {
	return &App{
		torrents:       make(map[string]*torrent.Torrent),
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

	a.wallet, err = createWallet()
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	// _, err = os.Stat(filepath.Join(homeDir, "wif.txt"))
	// if err == os.ErrNotExist {
	// 	a.wallet, err = createWallet()
	// 	if err != nil {
	// 		log.Default().Fatalf("Error: %s\n", err.Error())
	// 	}
	// } else {
	// 	a.wallet, err = loadWallet(filepath.Join(homeDir, "wif.txt"))
	// 	if err != nil {
	// 		log.Default().Fatalf("Error: %s\n", err.Error())
	// 	}
	// }

	err = os.MkdirAll(a.downloadDir, 0755)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

	err = os.MkdirAll(a.piecesDir, 0755)
	if err != nil {
		log.Default().Fatalf("Error: %s\n", err.Error())
	}

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
									m.PeerConn.ReleaseRequest()
								}
							}
						}

					case "REQUEST":
						{
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
								return
							}

							if totalInputAmount > totalSpentAmount {
								microTransaction.AddOutput(&transaction.TransactionOutput{
									Satoshis:      totalInputAmount - totalSpentAmount,
									LockingScript: a.wallet.LockingScript,
								})

								a.wallet.WalletUtxos = []UTXO{
									{0, (totalInputAmount - totalSpentAmount), microTransaction.TxID().String()},
								}
							}

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
		var timer = time.Tick(5 * time.Second)

		for range timer {
			a.appStateLocker.Lock()
			a.updateStatsLoop()
			a.appStateLocker.Unlock()
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
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	if a.client == nil {
		return fmt.Errorf("torrent client not initialized")
	}

	t, err := a.client.AddMagnet(magnetURI)
	if err != nil {
		return fmt.Errorf("failed to add magnet: %w", err)
	}

	var infoHash string = t.InfoHash().String()

	a.speedStatsLocker.Lock()
	a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
	a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
	a.speedStatsLocker.Unlock()

	a.torrents[infoHash] = t

	wailsruntime.EventsEmit(a.ctx, "torrent-added", infoHash)

	go func() {
		<-t.GotInfo()
		t.AllowDataDownload()
		t.AllowDataUpload()
		t.DownloadAll()

		a.appStateLocker.RLock()
		a.saveTorrentsState()
		a.appStateLocker.RUnlock()

		wailsruntime.EventsEmit(a.ctx, "torrent-updated", infoHash)
	}()

	return nil
}

func (a *App) CreateTorrentFromPath(path string) (*string, error) {
	a.appStateLocker.Lock()
	defer a.appStateLocker.Unlock()

	a.speedStatsLocker.Lock()
	defer a.speedStatsLocker.Lock()

	if a.client == nil {
		return nil, fmt.Errorf("torrent client not initialized")
	}

	var metaInfo metainfo.MetaInfo = metainfo.MetaInfo{
		AnnounceList: builtinAnnounceList,
		CreationDate: time.Now().Unix(),
	}

	totalLength, err := totalLength(path)
	if err != nil {
		return nil, err
	}

	var info = metainfo.Info{
		PieceLength: metainfo.ChoosePieceLength(totalLength),
	}

	err = info.BuildFromFilePath(path)
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

	defer pieceInformationStorage.Close()

	t, _ := a.client.AddTorrentOpt(torrent.AddTorrentOpts{
		InfoBytes: metaInfo.InfoBytes,
		InfoHash:  metaInfo.HashInfoBytes(),
		Storage: storage.NewFileOpts(storage.NewFileClientOpts{
			ClientBaseDir: a.downloadDir,
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

	wailsruntime.EventsEmit(a.ctx, "torrent-added", infoHash)
	a.saveTorrentsState()

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

	var torrents []*SeedRushTorrentInfo
	for hash := range a.torrents {
		info, err := a.getTorrentInfo(hash)
		if err != nil {
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

	t.AllowDataDownload()
	t.DownloadAll()
	a.saveTorrentsState()

	delete(a.pausedTorrents, infoHash)

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

	delete(a.torrents, infoHash)
	delete(a.pausedTorrents, infoHash)

	t.DisallowDataDownload()
	t.DisallowDataUpload()

	delete(a.downloadSpeeds, infoHash)
	delete(a.uploadSpeeds, infoHash)

	if deleteFiles && t.Info() != nil {
		for _, file := range t.Files() {
			err := os.Remove(filepath.Join(a.downloadDir, file.Path()))
			if err != nil {
				log.Default().Printf("Error: %s\n", err.Error())
			}
		}
	}

	t.Drop()
	a.saveTorrentsState()

	return nil
}

func (a *App) GetStats() Stats {
	a.appStateLocker.RLock()
	defer a.appStateLocker.RUnlock()

	a.speedStatsLocker.RLock()
	defer a.speedStatsLocker.RUnlock()

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

	return Stats{
		TotalDownloadSpeed: formatSpeed(totalDown),
		TotalUploadSpeed:   formatSpeed(totalUp),
		ActiveTorrents:     activeTorrents,
		TotalPeers:         activePeers,
	}
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

func (a *App) saveTorrentsState() {
	var states []TorrentState
	for hash, t := range a.torrents {
		var magnetURI string
		if t.Info() != nil {
			var metaInfo = t.Metainfo()
			magnetLink, _ := metaInfo.MagnetV2()
			magnetURI = magnetLink.String()
		}

		states = append(states, TorrentState{
			IsPaused:  a.pausedTorrents[hash],
			InfoHash:  hash,
			MagnetURI: magnetURI,
			UpdatedAt: time.Now(),
		})
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
	a.speedStatsLocker.Lock()
	defer a.speedStatsLocker.Unlock()

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
			log.Default().Printf("Error: %s\n", err.Error())
			continue
		}

		<-t.GotInfo()

		var infoHash = t.InfoHash().String()

		a.downloadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}
		a.uploadSpeeds[infoHash] = &speedTracker{lastTime: time.Now()}

		a.torrents[infoHash] = t

		if states[i].IsPaused {
			a.pausedTorrents[infoHash] = true
		} else {
			go func() {
				t.AllowDataDownload()
				t.AllowDataUpload()
				t.DownloadAll()
			}()
		}
	}
}

func (a *App) getTorrentInfo(hash string) (*SeedRushTorrentInfo, error) {
	a.speedStatsLocker.RLock()
	defer a.speedStatsLocker.RUnlock()

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
		IsPaused:      isPaused,
		Peers:         stats.ActivePeers,
		Seeds:         stats.ConnectedSeeders,
		DownloadSpeed: downloadSpeed,
		UploadSpeed:   uploadSpeed,
		Size:          t.Length(),
		Progress:      progress,
		ID:            hash,
		Name:          t.Name(),
		InfoHash:      hash,
		SizeStr:       formatBytes(t.Length()),
		Status:        a.getTorrentStatus(t, stats, isPaused),
		DownloadedStr: formatSpeed(downloadSpeed),
		UploadedStr:   formatSpeed(uploadSpeed),
		ETA:           eta,
		Files:         files,
		UpdatedAt:     time.Now(),
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

func (a *App) updateStatsLoop() {
	for hash, t := range a.torrents {
		var stats torrent.TorrentStats = t.Stats()
		var now time.Time = time.Now()

		if tracker, ok := a.downloadSpeeds[hash]; ok {
			elapsed := now.Sub(tracker.lastTime).Seconds()
			if elapsed > 0 {
				currentBytes := stats.BytesReadData.Int64()
				bytesDiff := currentBytes - tracker.lastBytes
				tracker.speed = int64(float64(bytesDiff) / elapsed)
				tracker.lastBytes = currentBytes
				tracker.lastTime = now
			}
		}

		if tracker, ok := a.uploadSpeeds[hash]; ok {
			elapsed := now.Sub(tracker.lastTime).Seconds()
			if elapsed > 0 {
				currentBytes := stats.BytesWrittenData.Int64()
				bytesDiff := currentBytes - tracker.lastBytes
				tracker.speed = int64(float64(bytesDiff) / elapsed)
				tracker.lastBytes = currentBytes
				tracker.lastTime = now
			}
		}
	}

	var torrents []*SeedRushTorrentInfo
	for hash := range a.torrents {
		info, err := a.getTorrentInfo(hash)
		if err != nil {
			return
		}

		torrents = append(torrents, info)
	}

	a.lastUpdateTime = time.Now()
	wailsruntime.EventsEmit(a.ctx, "torrents-update", map[string]interface{}{
		"torrents": torrents,
		"stats":    a.GetStats(),
	})
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
