export namespace updater {
	
	export class Status {
	    current: string;
	    latest: string;
	    update_available: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.update_available = source["update_available"];
	        this.message = source["message"];
	    }
	}

}

