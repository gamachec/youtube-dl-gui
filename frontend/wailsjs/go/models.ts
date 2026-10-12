export namespace config {
	
	export class Config {
	    outputDir: string;
	    lastMode: string;
	    version: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outputDir = source["outputDir"];
	        this.lastMode = source["lastMode"];
	        this.version = source["version"];
	    }
	}

}

export namespace ytdlp {
	
	export class VideoInfo {
	    id: string;
	    title: string;
	    uploader: string;
	    thumbnail: string;
	    webpageUrl: string;
	    duration: number;
	    isLive: boolean;
	    maxHeight: number;
	    hasAudio: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.uploader = source["uploader"];
	        this.thumbnail = source["thumbnail"];
	        this.webpageUrl = source["webpageUrl"];
	        this.duration = source["duration"];
	        this.isLive = source["isLive"];
	        this.maxHeight = source["maxHeight"];
	        this.hasAudio = source["hasAudio"];
	    }
	}

}

