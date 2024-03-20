export type HostSettings = {
	acceptingcontracts: boolean,
	maxdownloadbatchsize: number,
	maxduration: number,
	maxrevisebatchsize: number, 
	netaddress: string,
	remainingstorage: number,
	sectorsize: number,
	totalstorage: number,
	unlockhash: string,
	windowsize: number,
	collateral: string
	maxcollateral: string,
	baserpcprice: string,
	contractprice: string,
	downloadbandwidthprice: string,
	sectoraccessprice: string,
	storageprice: string,
	uploadbandwidthprice: string,
	ephemeralaccountexpiry: number,
	maxephemeralaccountbalance: string,
	revisionnumber: number,
	version: string,
	siamuxport: string
}

export type HostPriceTable = {
	uid: string,
	validity: number,
	hostblockheight: number,
	updatepricetablecost: string,
	accountbalancecost: string,
	fundaccountcost: string,
	latestrevisioncost: string,
	subscriptionmemorycost: string,
	subscriptionnotificationcost: string,
	initbasecost: string,
	memorytimecost: string,
	downloadbandwidthcost: string,
	uploadbandwidthcost: string,
	dropsectorsbasecost: string,
	dropsectorsunitcost: string,
	hassectorbasecost: string,
	readbasecost: string,
	readlengthcost: string,
	renewcontractcost: string,
	revisionbasecost: string,
	swapsectorcost: string,
	writebasecost: string,
	writelengthcost: string,
	writestorecost: string,
	txnfeeminrecommended: string,
	txnfeemaxrecommended: string,
	contractprice: string,
	collateralcost: string,
	maxcollateral: string,
	maxduration: number,
	windowsize: number,
	registryentriesleft: number,
	registryentriestotal: number
}

export type HostScan = {
	timestamp: string,
	success: boolean,
	latency: number,
	error: string,
	settings: HostSettings,
	priceTable: HostPriceTable
}

export type HostBenchmark = {
	timestamp: string,
	success: boolean,
	error: string,
	uploadSpeed: number,
	downloadSpeed: number,
	ttfb: number
}

export type HostInteractions = {
	uptime: number,
	downtime: number,
	scanHistory: HostScan[],
	lastBenchmark: HostBenchmark,
	lastSeen: string,
	activeHosts: number,
	historicSuccessfulInteractions: number,
	historicFailedInteractions: number,
	recentSuccessfulInteractions: number,
	recentFailedInteractions: number
}

export type Host = {
	id: number,
	publicKey: string,
	firstSeen: string,
	knownSince: number,
	netaddress: string,
	blocked: boolean,
	interactions: { [node: string]: HostInteractions },
	ipNets: string[],
	lastIPChange: string,
	settings: HostSettings,
	priceTable: HostPriceTable,
	ip: string,
	hostname: string,
	city: string,
	region: string,
	country: string,
	loc: string,
	org: string,
	postal: string,
	timezone: string
}