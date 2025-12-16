export namespace main {
	
	export class SeedRushFileInfo {
	    size: number;
	    progress: number;
	    name: string;
	    sizeStr: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SeedRushFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.size = source["size"];
	        this.progress = source["progress"];
	        this.name = source["name"];
	        this.sizeStr = source["sizeStr"];
	        this.path = source["path"];
	    }
	}
	export class SeedRushTorrentInfo {
	    isPaused: boolean;
	    peers: number;
	    seeds: number;
	    size: number;
	    downloadSpeed: number;
	    uploadSpeed: number;
	    satoshisSpend: number;
	    satoshisEarned: number;
	    progress: number;
	    id: string;
	    name: string;
	    infoHash: string;
	    sizeStr: string;
	    status: string;
	    downloadSpeedStr: string;
	    uploadSpeedStr: string;
	    eta: string;
	    // Go type: time
	    addedAt: any;
	    files: SeedRushFileInfo[];
	
	    static createFrom(source: any = {}) {
	        return new SeedRushTorrentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isPaused = source["isPaused"];
	        this.peers = source["peers"];
	        this.seeds = source["seeds"];
	        this.size = source["size"];
	        this.downloadSpeed = source["downloadSpeed"];
	        this.uploadSpeed = source["uploadSpeed"];
	        this.satoshisSpend = source["satoshisSpend"];
	        this.satoshisEarned = source["satoshisEarned"];
	        this.progress = source["progress"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.infoHash = source["infoHash"];
	        this.sizeStr = source["sizeStr"];
	        this.status = source["status"];
	        this.downloadSpeedStr = source["downloadSpeedStr"];
	        this.uploadSpeedStr = source["uploadSpeedStr"];
	        this.eta = source["eta"];
	        this.addedAt = this.convertValues(source["addedAt"], null);
	        this.files = this.convertValues(source["files"], SeedRushFileInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Stats {
	    totalDownload: string;
	    totalUpload: string;
	    activeTorrents: number;
	    totalPeers: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalDownload = source["totalDownload"];
	        this.totalUpload = source["totalUpload"];
	        this.activeTorrents = source["activeTorrents"];
	        this.totalPeers = source["totalPeers"];
	    }
	}
	export class WalletState {
	    balance: number;
	    address: string;
	
	    static createFrom(source: any = {}) {
	        return new WalletState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.balance = source["balance"];
	        this.address = source["address"];
	    }
	}

}

