export namespace k8sclient {
	
	export class AIContext {
	    text: string;
	    events: number;
	    logContainers: number;
	    logLines: number;
	    previousLogContainers: number;
	    hasYAML: boolean;
	    chars: number;
	
	    static createFrom(source: any = {}) {
	        return new AIContext(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.events = source["events"];
	        this.logContainers = source["logContainers"];
	        this.logLines = source["logLines"];
	        this.previousLogContainers = source["previousLogContainers"];
	        this.hasYAML = source["hasYAML"];
	        this.chars = source["chars"];
	    }
	}
	export class AccessSet {
	    kind: string;
	    namespace: string;
	    verbs: Record<string, boolean>;
	    checked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AccessSet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.verbs = source["verbs"];
	        this.checked = source["checked"];
	    }
	}
	export class PermissionRequirement {
	    kind: string;
	    namespace: string;
	    verb: string;
	    subresource: string;
	    allowed: boolean;
	    checked: boolean;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new PermissionRequirement(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.verb = source["verb"];
	        this.subresource = source["subresource"];
	        this.allowed = source["allowed"];
	        this.checked = source["checked"];
	        this.reason = source["reason"];
	    }
	}
	export class PermissionPlan {
	    operation: string;
	    requirements: PermissionRequirement[];
	    denied: number;
	    unknown: number;
	    permitted: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new PermissionPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.operation = source["operation"];
	        this.requirements = this.convertValues(source["requirements"], PermissionRequirement);
	        this.denied = source["denied"];
	        this.unknown = source["unknown"];
	        this.permitted = source["permitted"];
	        this.warnings = source["warnings"];
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
	export class ApplyDiffDoc {
	    ref: string;
	    action: string;
	    current: string;
	    proposed: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ApplyDiffDoc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.action = source["action"];
	        this.current = source["current"];
	        this.proposed = source["proposed"];
	        this.error = source["error"];
	    }
	}
	export class ApplyDiff {
	    docs: ApplyDiffDoc[];
	    create: number;
	    update: number;
	    unchanged: number;
	    failed: number;
	    permissions?: PermissionPlan;
	
	    static createFrom(source: any = {}) {
	        return new ApplyDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.docs = this.convertValues(source["docs"], ApplyDiffDoc);
	        this.create = source["create"];
	        this.update = source["update"];
	        this.unchanged = source["unchanged"];
	        this.failed = source["failed"];
	        this.permissions = this.convertValues(source["permissions"], PermissionPlan);
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
	
	export class CRDInfo {
	    name: string;
	    group: string;
	    kind: string;
	    scope: string;
	    versions: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new CRDInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.group = source["group"];
	        this.kind = source["kind"];
	        this.scope = source["scope"];
	        this.versions = source["versions"];
	        this.age = source["age"];
	    }
	}
	export class CheckFinding {
	    severity: string;
	    title: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new CheckFinding(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	    }
	}
	export class CertificateCheck {
	    source: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    hasCertificate: boolean;
	    subject: string;
	    dnsNames: string[];
	    issuer: string;
	    notAfter: string;
	    daysLeft: number;
	    expires: string;
	    usedBy: string[];
	    managedBy: string;
	    severity: string;
	    findings: CheckFinding[];
	
	    static createFrom(source: any = {}) {
	        return new CertificateCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.hasCertificate = source["hasCertificate"];
	        this.subject = source["subject"];
	        this.dnsNames = source["dnsNames"];
	        this.issuer = source["issuer"];
	        this.notAfter = source["notAfter"];
	        this.daysLeft = source["daysLeft"];
	        this.expires = source["expires"];
	        this.usedBy = source["usedBy"];
	        this.managedBy = source["managedBy"];
	        this.severity = source["severity"];
	        this.findings = this.convertValues(source["findings"], CheckFinding);
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
	export class CertificateReport {
	    certificates: CertificateCheck[];
	    critical: number;
	    warning: number;
	    certManagerInstalled: boolean;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new CertificateReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.certificates = this.convertValues(source["certificates"], CertificateCheck);
	        this.critical = source["critical"];
	        this.warning = source["warning"];
	        this.certManagerInstalled = source["certManagerInstalled"];
	        this.warnings = source["warnings"];
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
	export class ChartLink {
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new ChartLink(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}
	export class ChartDetail {
	    name: string;
	    repo: string;
	    repoURL: string;
	    version: string;
	    appVersion: string;
	    description: string;
	    homeURL: string;
	    readme: string;
	    defaultValues: string;
	    maintainers: string[];
	    keywords: string[];
	    links: ChartLink[];
	    versions: string[];
	
	    static createFrom(source: any = {}) {
	        return new ChartDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.repo = source["repo"];
	        this.repoURL = source["repoURL"];
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.description = source["description"];
	        this.homeURL = source["homeURL"];
	        this.readme = source["readme"];
	        this.defaultValues = source["defaultValues"];
	        this.maintainers = source["maintainers"];
	        this.keywords = source["keywords"];
	        this.links = this.convertValues(source["links"], ChartLink);
	        this.versions = source["versions"];
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
	
	export class ChartSearchResult {
	    name: string;
	    normalizedName: string;
	    repo: string;
	    repoURL: string;
	    sourceID: string;
	    version: string;
	    appVersion: string;
	    description: string;
	    stars: number;
	    versions?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ChartSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.normalizedName = source["normalizedName"];
	        this.repo = source["repo"];
	        this.repoURL = source["repoURL"];
	        this.sourceID = source["sourceID"];
	        this.version = source["version"];
	        this.appVersion = source["appVersion"];
	        this.description = source["description"];
	        this.stars = source["stars"];
	        this.versions = source["versions"];
	    }
	}
	
	export class StuckObject {
	    kind: string;
	    refKind: string;
	    apiVersion: string;
	    namespace: string;
	    name: string;
	    deletedAt: string;
	    terminating: string;
	    stuck: boolean;
	    finalizers: string[];
	    severity: string;
	    findings: CheckFinding[];
	    command: string;
	    commandNote: string;
	
	    static createFrom(source: any = {}) {
	        return new StuckObject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.refKind = source["refKind"];
	        this.apiVersion = source["apiVersion"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.deletedAt = source["deletedAt"];
	        this.terminating = source["terminating"];
	        this.stuck = source["stuck"];
	        this.finalizers = source["finalizers"];
	        this.severity = source["severity"];
	        this.findings = this.convertValues(source["findings"], CheckFinding);
	        this.command = source["command"];
	        this.commandNote = source["commandNote"];
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
	export class StuckReport {
	    objects: StuckObject[];
	    scanned: number;
	    failed: number;
	    critical: number;
	    warning: number;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new StuckReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.objects = this.convertValues(source["objects"], StuckObject);
	        this.scanned = source["scanned"];
	        this.failed = source["failed"];
	        this.critical = source["critical"];
	        this.warning = source["warning"];
	        this.warnings = source["warnings"];
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
	export class WebhookCheck {
	    configKind: string;
	    configuration: string;
	    webhook: string;
	    failurePolicy: string;
	    timeoutSeconds: number;
	    target: string;
	    serviceNamespace: string;
	    serviceName: string;
	    readyEndpoints: number;
	    rules: string;
	    scope: string;
	    affectedNamespaces: number;
	    severity: string;
	    findings: CheckFinding[];
	
	    static createFrom(source: any = {}) {
	        return new WebhookCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configKind = source["configKind"];
	        this.configuration = source["configuration"];
	        this.webhook = source["webhook"];
	        this.failurePolicy = source["failurePolicy"];
	        this.timeoutSeconds = source["timeoutSeconds"];
	        this.target = source["target"];
	        this.serviceNamespace = source["serviceNamespace"];
	        this.serviceName = source["serviceName"];
	        this.readyEndpoints = source["readyEndpoints"];
	        this.rules = source["rules"];
	        this.scope = source["scope"];
	        this.affectedNamespaces = source["affectedNamespaces"];
	        this.severity = source["severity"];
	        this.findings = this.convertValues(source["findings"], CheckFinding);
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
	export class WebhookReport {
	    webhooks: WebhookCheck[];
	    critical: number;
	    warning: number;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new WebhookReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.webhooks = this.convertValues(source["webhooks"], WebhookCheck);
	        this.critical = source["critical"];
	        this.warning = source["warning"];
	        this.warnings = source["warnings"];
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
	export class ClusterChecksReport {
	    scope: string;
	    checkedAt: string;
	    webhooks: WebhookReport;
	    certificates: CertificateReport;
	    stuck: StuckReport;
	
	    static createFrom(source: any = {}) {
	        return new ClusterChecksReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.checkedAt = source["checkedAt"];
	        this.webhooks = this.convertValues(source["webhooks"], WebhookReport);
	        this.certificates = this.convertValues(source["certificates"], CertificateReport);
	        this.stuck = this.convertValues(source["stuck"], StuckReport);
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
	export class ClusterRoleBindingInfo {
	    name: string;
	    roleRef: string;
	    subjects: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterRoleBindingInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.roleRef = source["roleRef"];
	        this.subjects = source["subjects"];
	        this.age = source["age"];
	    }
	}
	export class ClusterRoleInfo {
	    name: string;
	    rules: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ClusterRoleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rules = source["rules"];
	        this.age = source["age"];
	    }
	}
	export class StructurePod {
	    name: string;
	    namespace: string;
	    status: string;
	    ready: string;
	    restarts: number;
	    node: string;
	    isError: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StructurePod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.ready = source["ready"];
	        this.restarts = source["restarts"];
	        this.node = source["node"];
	        this.isError = source["isError"];
	    }
	}
	export class StructureWorkload {
	    kind: string;
	    name: string;
	    namespace: string;
	    status: string;
	    isError: boolean;
	    pods: StructurePod[];
	
	    static createFrom(source: any = {}) {
	        return new StructureWorkload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.isError = source["isError"];
	        this.pods = this.convertValues(source["pods"], StructurePod);
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
	export class StructureService {
	    name: string;
	    namespace: string;
	    type: string;
	    routes: string[];
	    warning: string;
	    workloads: StructureWorkload[];
	
	    static createFrom(source: any = {}) {
	        return new StructureService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.type = source["type"];
	        this.routes = source["routes"];
	        this.warning = source["warning"];
	        this.workloads = this.convertValues(source["workloads"], StructureWorkload);
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
	export class StructureEntry {
	    kind: string;
	    refKind: string;
	    name: string;
	    namespace: string;
	    warning: string;
	    services: StructureService[];
	
	    static createFrom(source: any = {}) {
	        return new StructureEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.refKind = source["refKind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.warning = source["warning"];
	        this.services = this.convertValues(source["services"], StructureService);
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
	export class StructureSummary {
	    nodes: number;
	    namespaces: number;
	    pods: number;
	    unhealthy: number;
	
	    static createFrom(source: any = {}) {
	        return new StructureSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = source["nodes"];
	        this.namespaces = source["namespaces"];
	        this.pods = source["pods"];
	        this.unhealthy = source["unhealthy"];
	    }
	}
	export class ClusterStructureData {
	    scope: string;
	    summary: StructureSummary;
	    entries: StructureEntry[];
	    internal: StructureService[];
	    unexposed: StructureWorkload[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ClusterStructureData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.summary = this.convertValues(source["summary"], StructureSummary);
	        this.entries = this.convertValues(source["entries"], StructureEntry);
	        this.internal = this.convertValues(source["internal"], StructureService);
	        this.unexposed = this.convertValues(source["unexposed"], StructureWorkload);
	        this.warnings = source["warnings"];
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
	export class ConfigMapInfo {
	    namespace: string;
	    name: string;
	    keys: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigMapInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.keys = source["keys"];
	        this.age = source["age"];
	    }
	}
	export class ContainerSizing {
	    namespace: string;
	    pod: string;
	    container: string;
	    qos: string;
	    cpuRequest: number;
	    cpuLimit: number;
	    memRequest: number;
	    memLimit: number;
	    cpuUsage: number;
	    memUsage: number;
	    metricsObserved: boolean;
	    cpuPct: number;
	    memPct: number;
	    memOfLimit: number;
	    restarts: number;
	    oomKilled: boolean;
	    findings: string[];
	    severity: number;
	
	    static createFrom(source: any = {}) {
	        return new ContainerSizing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.pod = source["pod"];
	        this.container = source["container"];
	        this.qos = source["qos"];
	        this.cpuRequest = source["cpuRequest"];
	        this.cpuLimit = source["cpuLimit"];
	        this.memRequest = source["memRequest"];
	        this.memLimit = source["memLimit"];
	        this.cpuUsage = source["cpuUsage"];
	        this.memUsage = source["memUsage"];
	        this.metricsObserved = source["metricsObserved"];
	        this.cpuPct = source["cpuPct"];
	        this.memPct = source["memPct"];
	        this.memOfLimit = source["memOfLimit"];
	        this.restarts = source["restarts"];
	        this.oomKilled = source["oomKilled"];
	        this.findings = source["findings"];
	        this.severity = source["severity"];
	    }
	}
	export class ContainerState {
	    name: string;
	    ready: boolean;
	    state: string;
	    restartCount: number;
	    lastTermination: string;
	    lastTerminationAge: string;
	    hasPrevious: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContainerState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.state = source["state"];
	        this.restartCount = source["restartCount"];
	        this.lastTermination = source["lastTermination"];
	        this.lastTerminationAge = source["lastTerminationAge"];
	        this.hasPrevious = source["hasPrevious"];
	    }
	}
	export class CronJobInfo {
	    namespace: string;
	    name: string;
	    schedule: string;
	    suspend: boolean;
	    active: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new CronJobInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.schedule = source["schedule"];
	        this.suspend = source["suspend"];
	        this.active = source["active"];
	        this.age = source["age"];
	    }
	}
	export class CustomKind {
	    refKind: string;
	    kind: string;
	    group: string;
	    title: string;
	    namespaced: boolean;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomKind(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refKind = source["refKind"];
	        this.kind = source["kind"];
	        this.group = source["group"];
	        this.title = source["title"];
	        this.namespaced = source["namespaced"];
	        this.count = source["count"];
	    }
	}
	export class CustomKindList {
	    kinds: CustomKind[];
	    total: number;
	    overflow: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomKindList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kinds = this.convertValues(source["kinds"], CustomKind);
	        this.total = source["total"];
	        this.overflow = source["overflow"];
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
	export class CustomObject {
	    namespace: string;
	    name: string;
	    age: string;
	    status: string;
	    isError: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CustomObject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.age = source["age"];
	        this.status = source["status"];
	        this.isError = source["isError"];
	    }
	}
	export class ResourcePageMeta {
	    continue: string;
	    remaining: number;
	
	    static createFrom(source: any = {}) {
	        return new ResourcePageMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.continue = source["continue"];
	        this.remaining = source["remaining"];
	    }
	}
	export class CustomObjectPage {
	    items: CustomObject[];
	    page: ResourcePageMeta;
	
	    static createFrom(source: any = {}) {
	        return new CustomObjectPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], CustomObject);
	        this.page = this.convertValues(source["page"], ResourcePageMeta);
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
	export class DaemonSetInfo {
	    namespace: string;
	    name: string;
	    desired: number;
	    ready: number;
	    available: number;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new DaemonSetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.desired = source["desired"];
	        this.ready = source["ready"];
	        this.available = source["available"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class DeploymentInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    upToDate: number;
	    available: number;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new DeploymentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.upToDate = source["upToDate"];
	        this.available = source["available"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class DetailField {
	    label: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new DetailField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.value = source["value"];
	    }
	}
	export class DrainBudget {
	    namespace: string;
	    name: string;
	    allowedDisruptions: number;
	    podsOnNode: number;
	
	    static createFrom(source: any = {}) {
	        return new DrainBudget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.allowedDisruptions = source["allowedDisruptions"];
	        this.podsOnNode = source["podsOnNode"];
	    }
	}
	export class DrainImpact {
	    node: string;
	    evict: number;
	    daemonSetPods: number;
	    mirrorPods: number;
	    unmanaged: string[];
	    emptyDir: string[];
	    blockingBudgets: DrainBudget[];
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new DrainImpact(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.node = source["node"];
	        this.evict = source["evict"];
	        this.daemonSetPods = source["daemonSetPods"];
	        this.mirrorPods = source["mirrorPods"];
	        this.unmanaged = source["unmanaged"];
	        this.emptyDir = source["emptyDir"];
	        this.blockingBudgets = this.convertValues(source["blockingBudgets"], DrainBudget);
	        this.warnings = source["warnings"];
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
	export class EventInfo {
	    type: string;
	    reason: string;
	    message: string;
	    count: number;
	    age: string;
	    isWarn: boolean;
	    object: string;
	
	    static createFrom(source: any = {}) {
	        return new EventInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.reason = source["reason"];
	        this.message = source["message"];
	        this.count = source["count"];
	        this.age = source["age"];
	        this.isWarn = source["isWarn"];
	        this.object = source["object"];
	    }
	}
	export class FlowEntryPolicy {
	    verdict: string;
	    source: string;
	    blocked: number;
	    total: number;
	    policies: string[];
	
	    static createFrom(source: any = {}) {
	        return new FlowEntryPolicy(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.verdict = source["verdict"];
	        this.source = source["source"];
	        this.blocked = source["blocked"];
	        this.total = source["total"];
	        this.policies = source["policies"];
	    }
	}
	export class FlowPod {
	    name: string;
	    namespace: string;
	    status: string;
	    ready: string;
	    restarts: number;
	    node: string;
	    ip: string;
	    ownerKind: string;
	    ownerName: string;
	    isError: boolean;
	    isReady: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FlowPod(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.ready = source["ready"];
	        this.restarts = source["restarts"];
	        this.node = source["node"];
	        this.ip = source["ip"];
	        this.ownerKind = source["ownerKind"];
	        this.ownerName = source["ownerName"];
	        this.isError = source["isError"];
	        this.isReady = source["isReady"];
	    }
	}
	export class FlowService {
	    name: string;
	    namespace: string;
	    type: string;
	    clusterIP: string;
	    ports: string[];
	    routes: string[];
	    pods: FlowPod[];
	    readyPods: number;
	    warning: string;
	    entryPolicy?: FlowEntryPolicy;
	    via: string;
	    viaKind: string;
	    viaName: string;
	    viaNamespace: string;
	
	    static createFrom(source: any = {}) {
	        return new FlowService(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.type = source["type"];
	        this.clusterIP = source["clusterIP"];
	        this.ports = source["ports"];
	        this.routes = source["routes"];
	        this.pods = this.convertValues(source["pods"], FlowPod);
	        this.readyPods = source["readyPods"];
	        this.warning = source["warning"];
	        this.entryPolicy = this.convertValues(source["entryPolicy"], FlowEntryPolicy);
	        this.via = source["via"];
	        this.viaKind = source["viaKind"];
	        this.viaName = source["viaName"];
	        this.viaNamespace = source["viaNamespace"];
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
	export class FlowIngress {
	    kind: string;
	    refKind: string;
	    name: string;
	    namespace: string;
	    class: string;
	    address: string;
	    hosts: string[];
	    ports: string[];
	    tls: boolean;
	    services: FlowService[];
	    warning: string;
	    entryNote: string;
	
	    static createFrom(source: any = {}) {
	        return new FlowIngress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.refKind = source["refKind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.class = source["class"];
	        this.address = source["address"];
	        this.hosts = source["hosts"];
	        this.ports = source["ports"];
	        this.tls = source["tls"];
	        this.services = this.convertValues(source["services"], FlowService);
	        this.warning = source["warning"];
	        this.entryNote = source["entryNote"];
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
	
	export class FlowPodPolicies {
	    ingress: string[];
	    egress: string[];
	
	    static createFrom(source: any = {}) {
	        return new FlowPodPolicies(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ingress = source["ingress"];
	        this.egress = source["egress"];
	    }
	}
	
	export class HPAInfo {
	    namespace: string;
	    name: string;
	    target: string;
	    replicas: string;
	    minMax: string;
	    metrics: string;
	    status: string;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new HPAInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.target = source["target"];
	        this.replicas = source["replicas"];
	        this.minMax = source["minMax"];
	        this.metrics = source["metrics"];
	        this.status = source["status"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class HelmDiff {
	    current: string;
	    proposed: string;
	    chartDigest: string;
	    releaseRevision: number;
	    valuesDigest: string;
	    permissions?: PermissionPlan;
	
	    static createFrom(source: any = {}) {
	        return new HelmDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.proposed = source["proposed"];
	        this.chartDigest = source["chartDigest"];
	        this.releaseRevision = source["releaseRevision"];
	        this.valuesDigest = source["valuesDigest"];
	        this.permissions = this.convertValues(source["permissions"], PermissionPlan);
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
	export class HelmReleaseDetail {
	    name: string;
	    namespace: string;
	    revision: number;
	    status: string;
	    chart: string;
	    appVersion: string;
	    notes: string;
	    values: string;
	    manifest: string;
	
	    static createFrom(source: any = {}) {
	        return new HelmReleaseDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.chart = source["chart"];
	        this.appVersion = source["appVersion"];
	        this.notes = source["notes"];
	        this.values = source["values"];
	        this.manifest = source["manifest"];
	    }
	}
	export class HelmReleaseInfo {
	    namespace: string;
	    name: string;
	    revision: string;
	    status: string;
	    updated: string;
	    secretName: string;
	    isError: boolean;
	    isPending: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HelmReleaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.updated = source["updated"];
	        this.secretName = source["secretName"];
	        this.isError = source["isError"];
	        this.isPending = source["isPending"];
	    }
	}
	export class HelmResource {
	    apiVersion: string;
	    kind: string;
	    refKind: string;
	    name: string;
	    namespace: string;
	    status: string;
	    health: string;
	    ready: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HelmResource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.apiVersion = source["apiVersion"];
	        this.kind = source["kind"];
	        this.refKind = source["refKind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.health = source["health"];
	        this.ready = source["ready"];
	    }
	}
	export class HelmReleaseSnapshot {
	    detail?: HelmReleaseDetail;
	    resources: HelmResource[];
	
	    static createFrom(source: any = {}) {
	        return new HelmReleaseSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.detail = this.convertValues(source["detail"], HelmReleaseDetail);
	        this.resources = this.convertValues(source["resources"], HelmResource);
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
	export class HelmRepo {
	    name: string;
	    url: string;
	    authenticated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HelmRepo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.authenticated = source["authenticated"];
	    }
	}
	
	export class HelmRevision {
	    revision: number;
	    status: string;
	    chart: string;
	    updated: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new HelmRevision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.status = source["status"];
	        this.chart = source["chart"];
	        this.updated = source["updated"];
	        this.description = source["description"];
	    }
	}
	export class IngressInfo {
	    namespace: string;
	    name: string;
	    class: string;
	    hosts: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new IngressInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.class = source["class"];
	        this.hosts = source["hosts"];
	        this.age = source["age"];
	    }
	}
	export class JobInfo {
	    namespace: string;
	    name: string;
	    completions: string;
	    succeeded: number;
	    active: number;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new JobInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.completions = source["completions"];
	        this.succeeded = source["succeeded"];
	        this.active = source["active"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class LimitRangeInfo {
	    namespace: string;
	    name: string;
	    limits: number;
	    types: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new LimitRangeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.limits = source["limits"];
	        this.types = source["types"];
	        this.age = source["age"];
	    }
	}
	export class NamespaceInfo {
	    name: string;
	    status: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new NamespaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.age = source["age"];
	    }
	}
	export class QuotaLine {
	    quota: string;
	    resource: string;
	    hard: string;
	    used: string;
	
	    static createFrom(source: any = {}) {
	        return new QuotaLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.quota = source["quota"];
	        this.resource = source["resource"];
	        this.hard = source["hard"];
	        this.used = source["used"];
	    }
	}
	export class NamespaceSizing {
	    namespace: string;
	    containers: number;
	    pods: number;
	    cpuRequest: number;
	    cpuLimit: number;
	    cpuUsage: number;
	    memRequest: number;
	    memLimit: number;
	    memUsage: number;
	    metricsExpected: number;
	    metricsObserved: number;
	    noCpuRequest: number;
	    noMemRequest: number;
	    noMemLimit: number;
	    undeclared: number;
	    quota: QuotaLine[];
	    limitRanges: number;
	
	    static createFrom(source: any = {}) {
	        return new NamespaceSizing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.containers = source["containers"];
	        this.pods = source["pods"];
	        this.cpuRequest = source["cpuRequest"];
	        this.cpuLimit = source["cpuLimit"];
	        this.cpuUsage = source["cpuUsage"];
	        this.memRequest = source["memRequest"];
	        this.memLimit = source["memLimit"];
	        this.memUsage = source["memUsage"];
	        this.metricsExpected = source["metricsExpected"];
	        this.metricsObserved = source["metricsObserved"];
	        this.noCpuRequest = source["noCpuRequest"];
	        this.noMemRequest = source["noMemRequest"];
	        this.noMemLimit = source["noMemLimit"];
	        this.undeclared = source["undeclared"];
	        this.quota = this.convertValues(source["quota"], QuotaLine);
	        this.limitRanges = source["limitRanges"];
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
	export class NavCount {
	    view: string;
	    count: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new NavCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.view = source["view"];
	        this.count = source["count"];
	        this.errors = source["errors"];
	    }
	}
	export class NetworkFlows {
	    ingresses: FlowIngress[];
	    services: FlowService[];
	    routedCount: number;
	    endpointCount: number;
	    brokenCount: number;
	    scope: string;
	    warnings: string[];
	    policiesAvailable: boolean;
	    policyCount: number;
	    isolatedPods: number;
	    podPolicies: Record<string, FlowPodPolicies>;
	
	    static createFrom(source: any = {}) {
	        return new NetworkFlows(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ingresses = this.convertValues(source["ingresses"], FlowIngress);
	        this.services = this.convertValues(source["services"], FlowService);
	        this.routedCount = source["routedCount"];
	        this.endpointCount = source["endpointCount"];
	        this.brokenCount = source["brokenCount"];
	        this.scope = source["scope"];
	        this.warnings = source["warnings"];
	        this.policiesAvailable = source["policiesAvailable"];
	        this.policyCount = source["policyCount"];
	        this.isolatedPods = source["isolatedPods"];
	        this.podPolicies = this.convertValues(source["podPolicies"], FlowPodPolicies, true);
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
	export class NetworkPolicyInfo {
	    namespace: string;
	    name: string;
	    podSelector: string;
	    policyTypes: string;
	    effect: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new NetworkPolicyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.podSelector = source["podSelector"];
	        this.policyTypes = source["policyTypes"];
	        this.effect = source["effect"];
	        this.age = source["age"];
	    }
	}
	export class NodeInfo {
	    name: string;
	    status: string;
	    ready: boolean;
	    role: string;
	    version: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new NodeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.ready = source["ready"];
	        this.role = source["role"];
	        this.version = source["version"];
	        this.age = source["age"];
	    }
	}
	export class NodeMetric {
	    name: string;
	    cpuMilli: number;
	    memMi: number;
	    cpuCapacity: number;
	    memCapacity: number;
	
	    static createFrom(source: any = {}) {
	        return new NodeMetric(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.cpuMilli = source["cpuMilli"];
	        this.memMi = source["memMi"];
	        this.cpuCapacity = source["cpuCapacity"];
	        this.memCapacity = source["memCapacity"];
	    }
	}
	export class NsKindCount {
	    kind: string;
	    view: string;
	    count: number;
	    errors: number;
	
	    static createFrom(source: any = {}) {
	        return new NsKindCount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.view = source["view"];
	        this.count = source["count"];
	        this.errors = source["errors"];
	    }
	}
	export class PodMetric {
	    namespace: string;
	    name: string;
	    cpuMilli: number;
	    memMi: number;
	
	    static createFrom(source: any = {}) {
	        return new PodMetric(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.cpuMilli = source["cpuMilli"];
	        this.memMi = source["memMi"];
	    }
	}
	export class OverviewNodeStatus {
	    name: string;
	    ready: boolean;
	    schedulable: boolean;
	    pods: number;
	    pressure: string[];
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new OverviewNodeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.schedulable = source["schedulable"];
	        this.pods = source["pods"];
	        this.pressure = source["pressure"];
	        this.version = source["version"];
	    }
	}
	export class PodInfo {
	    namespace: string;
	    name: string;
	    status: string;
	    ready: string;
	    restarts: number;
	    isError: boolean;
	    podIP: string;
	    node: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new PodInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.ready = source["ready"];
	        this.restarts = source["restarts"];
	        this.isError = source["isError"];
	        this.podIP = source["podIP"];
	        this.node = source["node"];
	        this.age = source["age"];
	    }
	}
	export class OverviewStats {
	    nodes: number;
	    namespaces: number;
	    pods: number;
	    deployments: number;
	    errors: number;
	    podsAvailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OverviewStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = source["nodes"];
	        this.namespaces = source["namespaces"];
	        this.pods = source["pods"];
	        this.deployments = source["deployments"];
	        this.errors = source["errors"];
	        this.podsAvailable = source["podsAvailable"];
	    }
	}
	export class OverviewData {
	    stats: OverviewStats;
	    failingPods: PodInfo[];
	    nodeMetrics: NodeMetric[];
	    nodeStatus: OverviewNodeStatus[];
	    topPods: PodMetric[];
	    events: EventInfo[];
	    warnings: string[];
	    sectionErrors: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new OverviewData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stats = this.convertValues(source["stats"], OverviewStats);
	        this.failingPods = this.convertValues(source["failingPods"], PodInfo);
	        this.nodeMetrics = this.convertValues(source["nodeMetrics"], NodeMetric);
	        this.nodeStatus = this.convertValues(source["nodeStatus"], OverviewNodeStatus);
	        this.topPods = this.convertValues(source["topPods"], PodMetric);
	        this.events = this.convertValues(source["events"], EventInfo);
	        this.warnings = source["warnings"];
	        this.sectionErrors = source["sectionErrors"];
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
	
	
	export class PDBInfo {
	    namespace: string;
	    name: string;
	    budget: string;
	    allowedDisruptions: number;
	    healthy: string;
	    status: string;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new PDBInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.budget = source["budget"];
	        this.allowedDisruptions = source["allowedDisruptions"];
	        this.healthy = source["healthy"];
	        this.status = source["status"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class PVCInfo {
	    namespace: string;
	    name: string;
	    status: string;
	    capacity: string;
	    storageClass: string;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new PVCInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.capacity = source["capacity"];
	        this.storageClass = source["storageClass"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	
	
	export class PersistentVolumeInfo {
	    name: string;
	    capacity: string;
	    accessModes: string;
	    status: string;
	    claim: string;
	    storageClass: string;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new PersistentVolumeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.capacity = source["capacity"];
	        this.accessModes = source["accessModes"];
	        this.status = source["status"];
	        this.claim = source["claim"];
	        this.storageClass = source["storageClass"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	
	
	export class PodsPage {
	    pods: PodInfo[];
	    metrics: PodMetric[];
	    page: ResourcePageMeta;
	
	    static createFrom(source: any = {}) {
	        return new PodsPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pods = this.convertValues(source["pods"], PodInfo);
	        this.metrics = this.convertValues(source["metrics"], PodMetric);
	        this.page = this.convertValues(source["page"], ResourcePageMeta);
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
	export class PodsSnapshot {
	    pods: PodInfo[];
	    metrics: PodMetric[];
	
	    static createFrom(source: any = {}) {
	        return new PodsSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pods = this.convertValues(source["pods"], PodInfo);
	        this.metrics = this.convertValues(source["metrics"], PodMetric);
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
	
	export class RelationNode {
	    kind: string;
	    name: string;
	    namespace: string;
	    status: string;
	    isError: boolean;
	    children: RelationNode[];
	
	    static createFrom(source: any = {}) {
	        return new RelationNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.status = source["status"];
	        this.isError = source["isError"];
	        this.children = this.convertValues(source["children"], RelationNode);
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
	export class ResourceDetail {
	    kind: string;
	    name: string;
	    namespace: string;
	    created: string;
	    age: string;
	    labels: Record<string, string>;
	    annotations: Record<string, string>;
	    info: DetailField[];
	
	    static createFrom(source: any = {}) {
	        return new ResourceDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.namespace = source["namespace"];
	        this.created = source["created"];
	        this.age = source["age"];
	        this.labels = source["labels"];
	        this.annotations = source["annotations"];
	        this.info = this.convertValues(source["info"], DetailField);
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
	
	export class ResourceQuotaInfo {
	    namespace: string;
	    name: string;
	    summary: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ResourceQuotaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.summary = source["summary"];
	        this.age = source["age"];
	    }
	}
	export class RoleBindingInfo {
	    namespace: string;
	    name: string;
	    roleRef: string;
	    subjects: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleBindingInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.roleRef = source["roleRef"];
	        this.subjects = source["subjects"];
	        this.age = source["age"];
	    }
	}
	export class RoleInfo {
	    namespace: string;
	    name: string;
	    rules: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new RoleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.rules = source["rules"];
	        this.age = source["age"];
	    }
	}
	export class RolloutRevision {
	    revision: number;
	    name: string;
	    images: string;
	    current: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new RolloutRevision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.revision = source["revision"];
	        this.name = source["name"];
	        this.images = source["images"];
	        this.current = source["current"];
	        this.age = source["age"];
	    }
	}
	export class SearchHit {
	    kind: string;
	    view: string;
	    namespace: string;
	    name: string;
	    isError: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.view = source["view"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.isError = source["isError"];
	    }
	}
	export class SecretEntry {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class SecretInfo {
	    namespace: string;
	    name: string;
	    type: string;
	    keys: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.keys = source["keys"];
	        this.age = source["age"];
	    }
	}
	export class ServiceAccountInfo {
	    namespace: string;
	    name: string;
	    secrets: number;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceAccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.secrets = source["secrets"];
	        this.age = source["age"];
	    }
	}
	export class ServiceInfo {
	    namespace: string;
	    name: string;
	    type: string;
	    clusterIP: string;
	    ports: string;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.clusterIP = source["clusterIP"];
	        this.ports = source["ports"];
	        this.age = source["age"];
	    }
	}
	export class SizingReport {
	    scope: string;
	    totals: NamespaceSizing;
	    namespaces: NamespaceSizing[];
	    containers: ContainerSizing[];
	    advice: string[];
	    allocCpu: number;
	    allocMem: number;
	    nodes: number;
	    cpuReservedPct: number;
	    cpuUsedPct: number;
	    memReservedPct: number;
	    memUsedPct: number;
	    metricsAvailable: boolean;
	    metricsComplete: boolean;
	    metricsExpected: number;
	    metricsObserved: number;
	    metricsCoveragePct: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new SizingReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.totals = this.convertValues(source["totals"], NamespaceSizing);
	        this.namespaces = this.convertValues(source["namespaces"], NamespaceSizing);
	        this.containers = this.convertValues(source["containers"], ContainerSizing);
	        this.advice = source["advice"];
	        this.allocCpu = source["allocCpu"];
	        this.allocMem = source["allocMem"];
	        this.nodes = source["nodes"];
	        this.cpuReservedPct = source["cpuReservedPct"];
	        this.cpuUsedPct = source["cpuUsedPct"];
	        this.memReservedPct = source["memReservedPct"];
	        this.memUsedPct = source["memUsedPct"];
	        this.metricsAvailable = source["metricsAvailable"];
	        this.metricsComplete = source["metricsComplete"];
	        this.metricsExpected = source["metricsExpected"];
	        this.metricsObserved = source["metricsObserved"];
	        this.metricsCoveragePct = source["metricsCoveragePct"];
	        this.note = source["note"];
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
	export class StatefulSetInfo {
	    namespace: string;
	    name: string;
	    ready: string;
	    isError: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new StatefulSetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.ready = source["ready"];
	        this.isError = source["isError"];
	        this.age = source["age"];
	    }
	}
	export class StorageClassInfo {
	    name: string;
	    provisioner: string;
	    reclaimPolicy: string;
	    isDefault: boolean;
	    age: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageClassInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provisioner = source["provisioner"];
	        this.reclaimPolicy = source["reclaimPolicy"];
	        this.isDefault = source["isDefault"];
	        this.age = source["age"];
	    }
	}
	
	
	
	
	
	
	
	export class TrafficPolicyRef {
	    namespace: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new TrafficPolicyRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	    }
	}
	export class TrafficDirectionVerdict {
	    isolated: boolean;
	    allowed: boolean;
	    selecting: TrafficPolicyRef[];
	    allowing: TrafficPolicyRef[];
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new TrafficDirectionVerdict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.isolated = source["isolated"];
	        this.allowed = source["allowed"];
	        this.selecting = this.convertValues(source["selecting"], TrafficPolicyRef);
	        this.allowing = this.convertValues(source["allowing"], TrafficPolicyRef);
	        this.reason = source["reason"];
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
	export class TrafficCheckTarget {
	    pod: string;
	    namespace: string;
	    ip: string;
	    port: string;
	    egress: TrafficDirectionVerdict;
	    ingress: TrafficDirectionVerdict;
	    allowed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrafficCheckTarget(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pod = source["pod"];
	        this.namespace = source["namespace"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.egress = this.convertValues(source["egress"], TrafficDirectionVerdict);
	        this.ingress = this.convertValues(source["ingress"], TrafficDirectionVerdict);
	        this.allowed = source["allowed"];
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
	export class TrafficCheckResult {
	    sourceNamespace: string;
	    sourcePod: string;
	    destinationKind: string;
	    destinationNamespace: string;
	    destinationName: string;
	    verdict: string;
	    summary: string;
	    targets: TrafficCheckTarget[];
	    limitations: string[];
	
	    static createFrom(source: any = {}) {
	        return new TrafficCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sourceNamespace = source["sourceNamespace"];
	        this.sourcePod = source["sourcePod"];
	        this.destinationKind = source["destinationKind"];
	        this.destinationNamespace = source["destinationNamespace"];
	        this.destinationName = source["destinationName"];
	        this.verdict = source["verdict"];
	        this.summary = source["summary"];
	        this.targets = this.convertValues(source["targets"], TrafficCheckTarget);
	        this.limitations = source["limitations"];
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

export namespace main {
	
	export class AIConfigView {
	    provider: string;
	    endpoint: string;
	    model: string;
	    language: string;
	    hasApiKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.language = source["language"];
	        this.hasApiKey = source["hasApiKey"];
	    }
	}
	export class AIMessage {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new AIMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class AIStatus {
	    configured: boolean;
	    provider: string;
	    label: string;
	    model: string;
	    local: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.provider = source["provider"];
	        this.label = source["label"];
	        this.model = source["model"];
	        this.local = source["local"];
	    }
	}
	export class ClusterInfo {
	    id: string;
	    name: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClusterInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.active = source["active"];
	    }
	}
	export class ContextsResult {
	    contexts: string[];
	    currentContext: string;
	
	    static createFrom(source: any = {}) {
	        return new ContextsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contexts = source["contexts"];
	        this.currentContext = source["currentContext"];
	    }
	}
	export class PortForwardInfo {
	    connectionId: string;
	    key: string;
	    kind: string;
	    namespace: string;
	    name: string;
	    podName: string;
	    localPort: number;
	    remotePort: number;
	    keepRunning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortForwardInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.key = source["key"];
	        this.kind = source["kind"];
	        this.namespace = source["namespace"];
	        this.name = source["name"];
	        this.podName = source["podName"];
	        this.localPort = source["localPort"];
	        this.remotePort = source["remotePort"];
	        this.keepRunning = source["keepRunning"];
	    }
	}
	export class RecentConnection {
	    name: string;
	    path: string;
	    context: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.context = source["context"];
	    }
	}

}

