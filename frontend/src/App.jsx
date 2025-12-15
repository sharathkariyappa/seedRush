import { useState, useEffect, useMemo } from 'react';
import { Play, Pause, Trash2, Plus, Download, Upload, Users, Settings, FolderOpen, Link, Search, X, FileUp, Clock, HardDrive, Check, AlertCircle, Copy } from 'lucide-react';
import { AddMagnet, GetTorrents, GetStats, PauseTorrent, ResumeTorrent, RemoveTorrent, OpenDownloadFolder } from '../wailsjs/go/main/App';
import { EventsOff, EventsOn } from '../wailsjs/runtime/runtime';
import { SelectSeedPath } from '../wailsjs/go/main/App';
import { CreateTorrentFromPath } from '../wailsjs/go/main/App';

const TorrentClient = () => {
  const [torrents, setTorrents] = useState([]);
  const [stats, setStats] = useState({
    totalDownload: '0 B/s',
    totalUpload: '0 B/s',
    activeTorrents: 0,
    totalPeers: 0
  });
  const [selectedTorrent, setSelectedTorrent] = useState(null);
  const [path, setPath] = useState();
  const [showAddModal, setShowAddModal] = useState(false);
  const [showLocalFilesModal, setShowLocalFilesModal] = useState(false);
  const [magnetLink, setMagnetLink] = useState('');
  const [filterStatus, setFilterStatus] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [generatedMagnetLink, setGeneratedMagnetLink] = useState('');
  const [confirmDialog, setConfirmDialog] = useState(null);

  useEffect(() => {
    let mounted = true;
    
    const handleTorrentsUpdate = (data) => {
      if (mounted) {
        setTorrents(data.torrents || []);
        setStats(data.stats);
      }
    };
    
    const handleTorrentAdded = () => {
      if (mounted) {
        loadTorrents();
      }
    };
    
    loadTorrents();
    
    EventsOn('torrents-update', handleTorrentsUpdate);
    EventsOn('torrent-added', handleTorrentAdded);
  
    return () => {
      mounted = false;
      EventsOff('torrents-update', handleTorrentsUpdate);
      EventsOff('torrent-added', handleTorrentAdded);
    };
  }, []);

  const loadTorrents = async () => {
    try {
      const result = await GetTorrents();
      setTorrents(result || []) ;
      const statsResult = await GetStats();
      setStats(statsResult);
    } catch (err) {
      console.error('Failed to load torrents:', err);
    }
  };

  const handleAddMagnet = async () => {
    if (!magnetLink.trim()) {
      setError('Please enter a magnet link');
      setTimeout(() => setError(''), 3000);
      return;
    }

    if (!magnetLink.startsWith('magnet:?')) {
      setError('Invalid magnet link format');
      setTimeout(() => setError(''), 3000);
      return;
    }

    setLoading(true);
    setError('');

    try {
      await AddMagnet(magnetLink);
      setMagnetLink('');
      setShowAddModal(false);
    } catch (err) {
      setError(err.message || 'Failed to add torrent');
      setTimeout(() => setError(''), 3000);
    } finally {
      setLoading(false);
    }
  };

  const handleSelectSeedPath = async () => {
    try {
      const path = await SelectSeedPath();
      if (path) {
        setPath(path);
        setShowLocalFilesModal(true);
        setError('');
      }
    } catch (err) {
      setError('Failed to select files');
      setTimeout(() => setError(''), 3000);
    }
  };

  const handleCreateTorrent = async () => {
    setLoading(true);
    setError('');

    try {
      const magnetLink = await CreateTorrentFromPath(path);
      setGeneratedMagnetLink(magnetLink);
      await loadTorrents();
    } catch (err) {
      setError(err.message || 'Failed to create torrent');
      setTimeout(() => setError(''), 3000);
    } finally {
      setLoading(false);
    }
  };

  const handleCloseLocalFilesModal = () => {
    setShowLocalFilesModal(false);
    setGeneratedMagnetLink('');
    setError('');
  };

  const handleToggleStatus = async (torrent) => {
    try {
      if (torrent.status === 'paused' || torrent.status === 'stalled' || torrent.isPaused) {
        await ResumeTorrent(torrent.infoHash);
      } else {
        await PauseTorrent(torrent.infoHash);
      }

      await loadTorrents();
    } catch (err) {
      console.error('Failed to toggle torrent:', err);
      setError('Failed to change torrent status');
      setTimeout(() => setError(''), 3000);
    }
  };

  const handleRemoveTorrent = (torrent, deleteFiles = false) => {
    setConfirmDialog({
      torrent,
      deleteFiles,
      message: deleteFiles 
        ? `Remove "${torrent.name}" and delete downloaded files?`
        : `Remove "${torrent.name}"?`
    });
  };
  
  const confirmRemoval = async () => {
    const { torrent, deleteFiles } = confirmDialog;
    setConfirmDialog(null);
    
    try {
      console.log('✓ Starting removal');
      
      if (selectedTorrent?.infoHash === torrent.infoHash) {
        setSelectedTorrent(null);
        console.log('✓ Closed details panel');
      }
      
      console.log('📡 Calling RemoveTorrent...');
      await RemoveTorrent(torrent.infoHash, deleteFiles);
      console.log('✓ RemoveTorrent completed');
      
      console.log('🔄 Loading torrents...');
      await loadTorrents();
      console.log('✓ Torrents reloaded');
    } catch (err) {
      console.error('❌ Failed to remove torrent:', err);
      setError(err.message || 'Failed to remove torrent');
      setTimeout(() => setError(''), 3000);
    }
  };

  const handleOpenFolder = async () => {
    try {
      await OpenDownloadFolder();
    } catch (err) {
      console.error('Failed to open folder:', err);
      setError('Failed to open download folder');
      setTimeout(() => setError(''), 3000);
    }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
  };

  const filteredTorrents = useMemo(() => {
    return torrents?.filter(t => {
        const matchesStatus = filterStatus === 'all' || t.status === filterStatus;
        const matchesSearch = t.name.toLowerCase().includes(searchQuery.toLowerCase());
        return matchesStatus && matchesSearch;
      })
      .sort((a, b) => {
        return a.id.localeCompare(b.id);
      });
  }, [torrents, filterStatus, searchQuery]);

  const getStatusColor = (status) => {
    switch(status) {
      case 'downloading':
        return 'bg-[#06E7ED]/20 text-[#06E7ED]';
      case 'seeding':
        return 'bg-green-500/20 text-green-300';
      case 'completed':
        return 'bg-blue-500/20 text-blue-300';
      case 'stalled':
        return 'bg-yellow-500/20 text-yellow-300';
      case 'paused':
        return 'bg-gray-500/20 text-gray-300';
      case 'loading':
        return 'bg-purple-500/20 text-purple-300';
      default:
        return 'bg-gray-500/20 text-gray-300';
    }
  };

  const getStatusDisplay = (status) => {
    switch(status) {
      case 'downloading':
        return 'Downloading';
      case 'seeding':
        return 'Seeding';
      case 'completed':
        return 'Completed';
      case 'stalled':
        return 'Stalled';
      case 'paused':
        return 'Paused';
      case 'loading':
        return 'Loading...';
      default:
        return status;
    }
  };

  return (
    <div className="h-screen flex flex-col bg-[#081B2A] text-white">
      {/* Top Bar */}
      <div className="bg-[#0E1F2D] px-6 py-4 border-b border-white/5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-[#06E7ED]/10 flex items-center justify-center">
              <img src="/loogo.png" alt="SeedRush Logo" className="w-10 h-10 rounded-md" />
            </div>
            <div>
              <h1 className="text-xl font-bold text-white">SeedRush</h1>
              <p className="text-xs text-gray-400">Earn while you seed</p>
            </div>
          </div>

          <div className="flex items-center gap-4">
            <div className="flex items-center gap-4 px-4 py-2 bg-[#081B2A]/50 rounded-lg border border-white/5">
              <div className="flex items-center gap-2">
                <Download className="w-4 h-4 text-[#06E7ED]" />
                <span className="text-sm font-medium">{stats.totalDownload}</span>
              </div>
              <div className="w-px h-4 bg-white/10"></div>
              <div className="flex items-center gap-2">
                <Upload className="w-4 h-4 text-[#06E7ED]" />
                <span className="text-sm font-medium">{stats.totalUpload}</span>
              </div>
            </div>

            <button 
              onClick={handleOpenFolder}
              className="p-2 hover:bg-white/10 rounded-lg transition-all"
              title="Open Downloads Folder"
            >
              <FolderOpen className="w-5 h-5" />
            </button>

            <button className="p-2 hover:bg-white/10 rounded-lg transition-all">
              <Settings className="w-5 h-5" />
            </button>
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        <div className="w-64 bg-[#0C2437] p-4 border-r border-white/5">
          <button
            onClick={() => setShowAddModal(true)}
            className="w-full bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg px-4 py-3 flex items-center justify-center gap-2 font-semibold transition-all shadow-lg shadow-cyan-500/20 mb-3"
          >
            <Plus className="w-5 h-5" />
            Add Torrent
          </button>

          <button
            onClick={handleSelectSeedPath}
            className="w-full bg-[#0E1F2D] hover:bg-white/5 text-white rounded-lg px-4 py-3 flex items-center justify-center gap-2 font-semibold transition-all border border-white/10 mb-6"
          >
            <FileUp className="w-5 h-5" />
            Share Local Files
          </button>

          <div className="space-y-2">
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider px-3 mb-3">
              Filters
            </h3>
            {[
              { key: 'all', label: 'All' },
              { key: 'downloading', label: 'Downloading' },
              { key: 'seeding', label: 'Seeding' },
              { key: 'completed', label: 'Completed' },
              { key: 'paused', label: 'Paused' },
              { key: 'stalled', label: 'Stalled' }
            ].map(({ key, label }) => (
              <button
                key={key}
                onClick={() => setFilterStatus(key)}
                className={`w-full px-3 py-2 rounded-lg text-left text-sm transition-all ${
                  filterStatus === key
                    ? 'bg-[#06E7ED]/10 text-[#06E7ED]'
                    : 'hover:bg-white/5 text-gray-300'
                }`}
              >
                <span>{label}</span>
                <span className="float-right text-xs text-gray-500">
                  {key === 'all' ? torrents?.length : torrents.filter(t => t.status === key).length}
                </span>
              </button>
            ))}
          </div>

          <div className="mt-8 p-4 bg-[#0E1F2D] rounded-lg border border-white/5">
            <h3 className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">
              Statistics
            </h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-400">Active</span>
                <span className="font-medium text-[#06E7ED]">{stats.activeTorrents}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-400">Total Peers</span>
                <span className="font-medium text-[#06E7ED]">{stats.totalPeers}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Torrent List */}
        <div className="flex-1 flex flex-col">
          {/* Search Bar */}
          <div className="p-4 bg-[#0E1F2D] border-b border-white/5">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
              <input
                type="text"
                placeholder="Search torrents..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-[#081B2A]/50 border border-white/5 rounded-lg pl-10 pr-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[#06E7ED] focus:border-transparent transition-all"
              />
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery('')}
                  className="absolute right-3 top-1/2 -translate-y-1/2 p-1 hover:bg-white/10 rounded"
                >
                  <X className="w-4 h-4 text-gray-400" />
                </button>
              )}
            </div>
          </div>

          {/* Torrent Items */}
          <div className="flex-1 overflow-y-auto p-4 space-y-3">
            {filteredTorrents.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-full text-gray-400">
                <Download className="w-16 h-16 mb-4 opacity-20" />
                <p className="text-lg font-medium">
                  {searchQuery ? 'No torrents found' : 'No torrents yet'}
                </p>
                <p className="text-sm">
                  {searchQuery ? 'Try a different search' : 'Add a torrent to get started'}
                </p>
              </div>
            ) : (
              filteredTorrents.map(torrent => (
                <div
                  key={torrent.id}
                  onClick={() => setSelectedTorrent(torrent)}
                  className={`bg-[#0E1F2D] rounded-xl p-4 transition-all cursor-pointer border will-change-auto ${
                    selectedTorrent?.id === torrent.id
                      ? 'ring-2 ring-[#06E7ED] shadow-lg shadow-cyan-500/20 border-[#06E7ED]'
                      : 'border-white/5 hover:border-white/10'
                  }`}
                  style={{ contain: 'layout' }}
                >
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex-1 min-w-0">
                      <h3 className="font-semibold text-white truncate mb-1">{torrent.name}</h3>
                      <div className="flex items-center gap-3 mt-1 text-xs text-gray-400">
                        <span className="flex items-center gap-1">
                          <HardDrive className="w-3 h-3" />
                          {torrent.sizeStr}
                        </span>
                        <span>•</span>
                        <span className="flex items-center gap-1">
                          <Users className="w-3 h-3" />
                          {torrent.peers}
                        </span>
                        {torrent.eta && torrent.eta !== 'Unknown' && (
                          <>
                            <span>•</span>
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              {torrent.eta}
                            </span>
                          </>
                        )}
                        <span>•</span>
                        <span className={`px-2 py-0.5 rounded ${getStatusColor(torrent.status)}`}>
                          {getStatusDisplay(torrent.status)}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 ml-4">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleToggleStatus(torrent);
                        }}
                        className="p-2 hover:bg-white/10 rounded-lg transition-all"
                        title={torrent.isPaused || torrent.status === 'paused' ? 'Resume' : 'Pause'}
                      >
                        {torrent.isPaused || torrent.status === 'paused' || torrent.status === 'stalled' ? (
                          <Play className="w-4 h-4 text-[#06E7ED]" />
                        ) : (
                          <Pause className="w-4 h-4 text-orange-400" />
                        )}
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleRemoveTorrent(torrent, false);
                        }}
                        className="p-2 hover:bg-red-500/20 rounded-lg transition-all"
                        title="Remove"
                      >
                        <Trash2 className="w-4 h-4 text-red-400" />
                      </button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-xs text-gray-400">
                      <span className="font-medium">{torrent.progress.toFixed(1)}%</span>
                    </div>
                    <div className="h-1.5 bg-white/10 rounded-full overflow-hidden">
                      <div
                        className="h-full bg-gradient-to-r from-[#06E7ED] to-[#05CDD3] rounded-full transition-all duration-500"
                        style={{ width: `${Math.min(torrent.progress, 100)}%` }}
                      />
                    </div>
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-[#06E7ED] flex items-center gap-1">
                        <Download className="w-3 h-3" />
                        {torrent.downloadSpeedStr}
                      </span>
                      <span className="text-green-400 flex items-center gap-1">
                        <Upload className="w-3 h-3" />
                        {torrent.uploadSpeedStr}
                      </span>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Details Panel */}
        {selectedTorrent && (
          <div className="w-96 bg-[#0C2437] p-6 overflow-y-auto border-l border-white/5">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-lg font-bold">Details</h2>
              <button
                onClick={() => setSelectedTorrent(null)}
                className="p-1 hover:bg-white/10 rounded transition-all"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-6">
              <div>
                <h3 className="text-sm font-semibold text-gray-400 mb-3">FILES</h3>
                <div className="space-y-2">
                  {selectedTorrent.files && selectedTorrent.files.length > 0 ? (
                    selectedTorrent.files.map((file, idx) => (
                      <div key={idx} className="bg-[#0E1F2D] rounded-lg p-3 border border-white/5">
                        <div className="flex items-center justify-between mb-2">
                          <span className="text-sm font-medium truncate flex-1" title={file.name}>
                            {file.name}
                          </span>
                          <span className="text-xs text-gray-400 ml-2">{file.sizeStr}</span>
                        </div>
                        <div className="flex items-center gap-2">
                          <div className="flex-1 h-1 bg-white/10 rounded-full overflow-hidden">
                            <div
                              className="h-full bg-[#06E7ED]"
                              style={{ width: `${Math.min(file.progress, 100)}%` }}
                            />
                          </div>
                          <span className="text-xs text-gray-500 min-w-[45px] text-right">
                            {file.progress.toFixed(0)}%
                          </span>
                        </div>
                      </div>
                    ))
                  ) : (
                    <div className="text-sm text-gray-400 bg-[#0E1F2D] rounded-lg p-4 text-center border border-white/5">
                      {selectedTorrent.name === 'Loading metadata...' 
                        ? 'Waiting for metadata...' 
                        : 'No file information available'}
                    </div>
                  )}
                </div>
              </div>

              <div>
                <h3 className="text-sm font-semibold text-gray-400 mb-3">INFORMATION</h3>
                <div className="space-y-3 text-sm bg-[#0E1F2D] rounded-lg p-4 border border-white/5">
                  <div className="flex justify-between">
                    <span className="text-gray-400">Status</span>
                    <span className={`font-medium capitalize px-2 py-0.5 rounded text-xs ${getStatusColor(selectedTorrent.status)}`}>
                      {getStatusDisplay(selectedTorrent.status)}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Size</span>
                    <span className="font-medium">{selectedTorrent.sizeStr}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Progress</span>
                    <span className="font-medium text-[#06E7ED]">
                      {selectedTorrent.progress.toFixed(1)}%
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Download</span>
                    <span className="font-medium text-[#06E7ED]">
                      {selectedTorrent.downloadSpeedStr}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Upload</span>
                    <span className="font-medium text-green-400">
                      {selectedTorrent.uploadSpeedStr}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Peers</span>
                    <span className="font-medium">{selectedTorrent.peers}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-gray-400">Seeds</span>
                    <span className="font-medium">{selectedTorrent.seeds}</span>
                  </div>
                  {selectedTorrent.eta && selectedTorrent.eta !== 'Unknown' && (
                    <div className="flex justify-between">
                      <span className="text-gray-400">ETA</span>
                      <span className="font-medium">{selectedTorrent.eta}</span>
                    </div>
                  )}
                </div>
              </div>

              <div className="space-y-2">
                <button 
                  onClick={handleOpenFolder}
                  className="w-full bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg py-2.5 text-sm font-semibold transition-all flex items-center justify-center gap-2"
                >
                  <FolderOpen className="w-4 h-4" />
                  Open Download Folder
                </button>
                <button 
                  onClick={() => handleRemoveTorrent(selectedTorrent, true)}
                  className="w-full bg-red-500/10 hover:bg-red-500/20 text-red-400 rounded-lg py-2.5 text-sm font-semibold transition-all flex items-center justify-center gap-2 border border-red-500/20"
                >
                  <Trash2 className="w-4 h-4" />
                  Remove & Delete Files
                </button>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Add Torrent Modal */}
      {showAddModal && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-[#0C2437] rounded-2xl p-6 w-full max-w-lg shadow-2xl border border-white/10">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-bold">Add Torrent</h2>
              <button
                onClick={() => {
                  setShowAddModal(false);
                  setError('');
                  setMagnetLink('');
                }}
                className="p-2 hover:bg-white/10 rounded-lg transition-all"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {error && (
              <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm flex items-start gap-2">
                <AlertCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="text-sm font-medium text-gray-300 mb-2 block">
                  Magnet Link
                </label>
                <div className="relative">
                  <Link className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
                  <input
                    type="text"
                    placeholder="magnet:?xt=urn:btih:..."
                    value={magnetLink}
                    onChange={(e) => setMagnetLink(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && magnetLink.trim()) {
                        handleAddMagnet();
                      }
                    }}
                    className="w-full bg-[#0E1F2D] border border-white/5 rounded-lg pl-10 pr-4 py-3 focus:outline-none focus:ring-2 focus:ring-[#06E7ED] focus:border-transparent transition-all"
                    disabled={loading}
                  />
                </div>
                <p className="text-xs text-gray-500 mt-2">
                  Press Enter to add
                </p>
              </div>
              <div className="flex gap-3">
                <button
                  onClick={handleAddMagnet}
                  disabled={loading || !magnetLink.trim()}
                  className="flex-1 bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg py-3 font-semibold transition-all shadow-lg shadow-cyan-500/20 disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {loading ? 'Adding...' : 'Add Magnet'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Share Local Files Modal */}
      {showLocalFilesModal && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-[#0C2437] rounded-2xl p-6 w-full max-w-2xl shadow-2xl border border-white/10">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-bold">Share Local Files</h2>
              <button
                onClick={handleCloseLocalFilesModal}
                className="p-2 hover:bg-white/10 rounded-lg transition-all"
                disabled={loading}
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {error && (
              <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-red-400 text-sm flex items-start gap-2">
                <AlertCircle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="space-y-4">
              {loading && (
                <div className="bg-[#06E7ED]/10 border border-[#06E7ED]/20 rounded-lg p-4">
                  <div className="flex items-center gap-3 mb-3">
                    <div className="animate-spin rounded-full h-5 w-5 border-2 border-[#06E7ED] border-t-transparent"></div>
                    <span className="text-sm font-medium text-[#06E7ED]">Creating torrent and generating magnet link...</span>
                  </div>
                  <p className="text-xs text-gray-400">
                    This may take a few moments depending on file size.
                  </p>
                </div>
              )}

              {generatedMagnetLink && (
                <div className="bg-green-500/10 border border-green-500/20 rounded-lg p-4">
                  <div className="flex items-center gap-2 mb-3">
                    <Check className="w-5 h-5 text-green-400" />
                    <span className="font-semibold text-green-300">Torrent Created Successfully!</span>
                  </div>
                  <div className="space-y-2">
                    <p className="text-sm text-gray-300">Your files are now being seeded. Share this magnet link:</p>
                    <div className="flex items-center gap-2">
                      <input
                        type="text"
                        value={generatedMagnetLink}
                        readOnly
                        className="flex-1 bg-[#0E1F2D] border border-white/5 rounded-lg px-3 py-2 text-sm text-gray-300 focus:outline-none"
                      />
                      <button
                        onClick={() => copyToClipboard(generatedMagnetLink)}
                        className="px-4 py-2 bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg transition-all flex items-center gap-2 text-sm font-medium"
                      >
                        <Copy className="w-4 h-4" />
                        Copy
                      </button>
                    </div>
                  </div>
                </div>
              )}

              <div className="flex gap-3">
                {!generatedMagnetLink ? (
                  <>
                    <button
                      onClick={handleCreateTorrent}
                      disabled={loading}
                      className="flex-1 bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg py-3 font-semibold transition-all shadow-lg shadow-cyan-500/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                    >
                      {loading ? (
                        <>
                          <div className="animate-spin rounded-full h-5 w-5 border-2 border-[#081B2A] border-t-transparent"></div>
                          Creating...
                        </>
                      ) : (
                        <>
                          <Upload className="w-5 h-5" />
                          Create & Share Torrent
                        </>
                      )}
                    </button>
                    <button
                      onClick={handleCloseLocalFilesModal}
                      disabled={loading}
                      className="px-6 bg-[#0E1F2D] hover:bg-white/5 border border-white/10 rounded-lg font-medium transition-all disabled:opacity-50"
                    >
                      Cancel
                    </button>
                  </>
                ) : (
                  <button
                    onClick={handleCloseLocalFilesModal}
                    className="flex-1 bg-[#06E7ED] hover:bg-[#05CDD3] text-[#081B2A] rounded-lg py-3 font-semibold transition-all"
                  >
                    Done
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Confirmation Modal */}
      {confirmDialog && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-[#0C2437] rounded-2xl p-6 w-full max-w-lg shadow-2xl border border-white/10">
            <div className="flex items-center justify-between mb-6">
              <h2 className="text-xl font-bold">Confirm Removal</h2>
              <button
                onClick={() => setConfirmDialog(null)}
                className="p-2 hover:bg-white/10 rounded-lg transition-all"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="mb-6">
              <div className="flex items-start gap-3 p-4 bg-red-500/10 border border-red-500/20 rounded-lg">
                <AlertCircle className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />
                <div>
                  <p className="text-gray-200 font-medium mb-1">
                    {confirmDialog.message}
                  </p>
                  {confirmDialog.deleteFiles && (
                    <p className="text-sm text-gray-400">
                      This will permanently delete all downloaded files.
                    </p>
                  )}
                </div>
              </div>
            </div>

            <div className="flex gap-3">
              <button
                onClick={() => setConfirmDialog(null)}
                className="flex-1 bg-[#0E1F2D] hover:bg-white/5 border border-white/10 rounded-lg py-3 font-semibold transition-all"
              >
                Cancel
              </button>
              <button
                onClick={confirmRemoval}
                className="flex-1 bg-red-500 hover:bg-red-600 text-white rounded-lg py-3 font-semibold transition-all shadow-lg shadow-red-500/20"
              >
                {confirmDialog.deleteFiles ? 'Remove & Delete Files' : 'Remove Torrent'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
};
export default TorrentClient;