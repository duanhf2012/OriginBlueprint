export namespace main {
	
	export class ConfigTableEntry {
	    id: string;
	    name: string;
	    extra?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigTableEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.extra = source["extra"];
	    }
	}
	export class ConfigTable {
	    key: string;
	    name?: string;
	    file: string;
	    sheet?: string;
	    rowCount: number;
	    idColumn: string;
	    nameColumn: string;
	    imported: boolean;
	    missing?: boolean;
	    warning?: string;
	    entries: ConfigTableEntry[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigTable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.file = source["file"];
	        this.sheet = source["sheet"];
	        this.rowCount = source["rowCount"];
	        this.idColumn = source["idColumn"];
	        this.nameColumn = source["nameColumn"];
	        this.imported = source["imported"];
	        this.missing = source["missing"];
	        this.warning = source["warning"];
	        this.entries = this.convertValues(source["entries"], ConfigTableEntry);
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
	export class ConfigTableFilter {
	    mode?: string;
	    column?: string;
	    text?: string;
	    rangeMin?: string;
	    rangeMax?: string;
	    compareOp?: string;
	    compareValue?: string;
	    extraMatchRow?: number;
	    extraKeyword?: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigTableFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.column = source["column"];
	        this.text = source["text"];
	        this.rangeMin = source["rangeMin"];
	        this.rangeMax = source["rangeMax"];
	        this.compareOp = source["compareOp"];
	        this.compareValue = source["compareValue"];
	        this.extraMatchRow = source["extraMatchRow"];
	        this.extraKeyword = source["extraKeyword"];
	    }
	}
	export class ConfigTableDataset {
	    key: string;
	    name: string;
	    file: string;
	    sheet?: string;
	    idMatchRow?: number;
	    nameMatchRow?: number;
	    idKeyword?: string;
	    nameKeyword?: string;
	    filter?: ConfigTableFilter;
	
	    static createFrom(source: any = {}) {
	        return new ConfigTableDataset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.file = source["file"];
	        this.sheet = source["sheet"];
	        this.idMatchRow = source["idMatchRow"];
	        this.nameMatchRow = source["nameMatchRow"];
	        this.idKeyword = source["idKeyword"];
	        this.nameKeyword = source["nameKeyword"];
	        this.filter = this.convertValues(source["filter"], ConfigTableFilter);
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
	export class ConfigTableDefaults {
	    idMatchRow: number;
	    nameMatchRow: number;
	    idKeyword: string;
	    nameKeywords: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigTableDefaults(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.idMatchRow = source["idMatchRow"];
	        this.nameMatchRow = source["nameMatchRow"];
	        this.idKeyword = source["idKeyword"];
	        this.nameKeywords = source["nameKeywords"];
	    }
	}
	
	
	export class legacyConfigTableRule {
	    key: string;
	    file: string;
	    sheet?: string;
	    idMatchRow?: number;
	    nameMatchRow?: number;
	    idKeyword?: string;
	    nameKeyword?: string;
	    filter?: ConfigTableFilter;
	
	    static createFrom(source: any = {}) {
	        return new legacyConfigTableRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.file = source["file"];
	        this.sheet = source["sheet"];
	        this.idMatchRow = source["idMatchRow"];
	        this.nameMatchRow = source["nameMatchRow"];
	        this.idKeyword = source["idKeyword"];
	        this.nameKeyword = source["nameKeyword"];
	        this.filter = this.convertValues(source["filter"], ConfigTableFilter);
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
	export class ConfigTablesSettings {
	    directories: string[];
	    defaults: ConfigTableDefaults;
	    datasets: ConfigTableDataset[];
	    tables?: legacyConfigTableRule[];
	
	    static createFrom(source: any = {}) {
	        return new ConfigTablesSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.directories = source["directories"];
	        this.defaults = this.convertValues(source["defaults"], ConfigTableDefaults);
	        this.datasets = this.convertValues(source["datasets"], ConfigTableDataset);
	        this.tables = this.convertValues(source["tables"], legacyConfigTableRule);
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
	export class FileResult {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new FileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}
	export class NodeReferenceResult {
	    name: string;
	    path: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeReferenceResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.count = source["count"];
	    }
	}
	export class ProjectSettingsResult {
	    path: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectSettingsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	    }
	}
	export class RecoverySnapshotResult {
	    path: string;
	    sourcePath?: string;
	    tabId?: string;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RecoverySnapshotResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.sourcePath = source["sourcePath"];
	        this.tabId = source["tabId"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class RuntimeNodeLoadError {
	    path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeNodeLoadError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.message = source["message"];
	    }
	}
	export class RuntimeNodeSchemaDocument {
	    path: string;
	    key: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeNodeSchemaDocument(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.key = source["key"];
	        this.content = source["content"];
	    }
	}
	export class RuntimeNodeSchemaDocumentLoadResult {
	    documents: RuntimeNodeSchemaDocument[];
	    errors: RuntimeNodeLoadError[];
	
	    static createFrom(source: any = {}) {
	        return new RuntimeNodeSchemaDocumentLoadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.documents = this.convertValues(source["documents"], RuntimeNodeSchemaDocument);
	        this.errors = this.convertValues(source["errors"], RuntimeNodeLoadError);
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
	export class ValidationIssue {
	    severity: string;
	    code: string;
	    message: string;
	    nodeId?: string;
	    nodeIds?: string[];
	    sourcePath?: string;
	    blocksSave?: boolean;
	    blocksRun?: boolean;
	    target?: string;
	
	    static createFrom(source: any = {}) {
	        return new ValidationIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.code = source["code"];
	        this.message = source["message"];
	        this.nodeId = source["nodeId"];
	        this.nodeIds = source["nodeIds"];
	        this.sourcePath = source["sourcePath"];
	        this.blocksSave = source["blocksSave"];
	        this.blocksRun = source["blocksRun"];
	        this.target = source["target"];
	    }
	}
	export class WorkspaceEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	    }
	}

}

