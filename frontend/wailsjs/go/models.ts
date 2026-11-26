export namespace engine {
	
	export class Plan {
	    mode: string;
	    profile: string;
	    duration: number;
	    cpu: boolean;
	    ram: boolean;
	    disk: boolean;
	    net: boolean;
	    kernel?: string;
	    ram_bytes?: number;
	    disk_dir?: string;
	    disk_raw?: string;
	    confirm_serial?: string;
	    disk_write_cap?: number;
	    disk_file_size?: number;
	    net_server?: string;
	    net_streams?: number;
	    thermal_margin?: number;
	    allow_battery?: boolean;
	    lang?: string;
	
	    static createFrom(source: any = {}) {
	        return new Plan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.profile = source["profile"];
	        this.duration = source["duration"];
	        this.cpu = source["cpu"];
	        this.ram = source["ram"];
	        this.disk = source["disk"];
	        this.net = source["net"];
	        this.kernel = source["kernel"];
	        this.ram_bytes = source["ram_bytes"];
	        this.disk_dir = source["disk_dir"];
	        this.disk_raw = source["disk_raw"];
	        this.confirm_serial = source["confirm_serial"];
	        this.disk_write_cap = source["disk_write_cap"];
	        this.disk_file_size = source["disk_file_size"];
	        this.net_server = source["net_server"];
	        this.net_streams = source["net_streams"];
	        this.thermal_margin = source["thermal_margin"];
	        this.allow_battery = source["allow_battery"];
	        this.lang = source["lang"];
	    }
	}

}

export namespace main {
	
	export class DiffView {
	    rows: report.DiffRow[];
	    mismatch: string;
	    html_path: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], report.DiffRow);
	        this.mismatch = source["mismatch"];
	        this.html_path = source["html_path"];
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
	export class DiskView {
	    name: string;
	    path: string;
	    model?: string;
	    serial?: string;
	    firmware?: string;
	    size: number;
	    bus?: string;
	    rotational?: boolean;
	    removable?: boolean;
	    partitions: number;
	    system: boolean;
	    mounted?: boolean;
	    size_text: string;
	    raw_eligible: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiskView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.firmware = source["firmware"];
	        this.size = source["size"];
	        this.bus = source["bus"];
	        this.rotational = source["rotational"];
	        this.removable = source["removable"];
	        this.partitions = source["partitions"];
	        this.system = source["system"];
	        this.mounted = source["mounted"];
	        this.size_text = source["size_text"];
	        this.raw_eligible = source["raw_eligible"];
	    }
	}
	export class FindingView {
	    sev: string;
	    comp: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new FindingView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sev = source["sev"];
	        this.comp = source["comp"];
	        this.text = source["text"];
	    }
	}
	export class KV {
	    k: string;
	    v: string;
	
	    static createFrom(source: any = {}) {
	        return new KV(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.k = source["k"];
	        this.v = source["v"];
	    }
	}
	export class MachineView {
	    lang: string;
	    version: string;
	    config: KV[];
	    sensor_level: number;
	    sensor_text: string;
	    sensor_details: string[];
	    temp_c: number;
	    freq_mhz: number;
	    kernels: string[];
	    disks: DiskView[];
	    has_battery: boolean;
	    admin: boolean;
	    prev_crash: string;
	    temp_dir: string;
	    reports_dir: string;
	    addresses: string[];
	
	    static createFrom(source: any = {}) {
	        return new MachineView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lang = source["lang"];
	        this.version = source["version"];
	        this.config = this.convertValues(source["config"], KV);
	        this.sensor_level = source["sensor_level"];
	        this.sensor_text = source["sensor_text"];
	        this.sensor_details = source["sensor_details"];
	        this.temp_c = source["temp_c"];
	        this.freq_mhz = source["freq_mhz"];
	        this.kernels = source["kernels"];
	        this.disks = this.convertValues(source["disks"], DiskView);
	        this.has_battery = source["has_battery"];
	        this.admin = source["admin"];
	        this.prev_crash = source["prev_crash"];
	        this.temp_dir = source["temp_dir"];
	        this.reports_dir = source["reports_dir"];
	        this.addresses = source["addresses"];
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
	export class ReportView {
	    id: string;
	    created: string;
	    mode: string;
	    mode_text: string;
	    verdict: string;
	    verdict_text: string;
	    verdict_long: string;
	    findings: FindingView[];
	    limits: string[];
	    elapsed: string;
	    host: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.created = source["created"];
	        this.mode = source["mode"];
	        this.mode_text = source["mode_text"];
	        this.verdict = source["verdict"];
	        this.verdict_text = source["verdict_text"];
	        this.verdict_long = source["verdict_long"];
	        this.findings = this.convertValues(source["findings"], FindingView);
	        this.limits = source["limits"];
	        this.elapsed = source["elapsed"];
	        this.host = source["host"];
	        this.path = source["path"];
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

}

export namespace report {
	
	export class DiffRow {
	    section: string;
	    metric: string;
	    unit: string;
	    a: number;
	    b: number;
	    has_a: boolean;
	    has_b: boolean;
	    delta_pct: number;
	    higher: boolean;
	    better: number;
	
	    static createFrom(source: any = {}) {
	        return new DiffRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.metric = source["metric"];
	        this.unit = source["unit"];
	        this.a = source["a"];
	        this.b = source["b"];
	        this.has_a = source["has_a"];
	        this.has_b = source["has_b"];
	        this.delta_pct = source["delta_pct"];
	        this.higher = source["higher"];
	        this.better = source["better"];
	    }
	}

}

