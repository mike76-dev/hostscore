export type HostPrices = {
	contractPrice: string,
	collateral: string,
	storagePrice: string,
	ingressPrice: string,
	egressPrice: string,
	freeSectorPrice: string
}

export type HostSettingsV2 = {
	protocolVersion: number[],
	release: string,
	walletAddress: string,
	acceptingContracts: boolean,
	maxCollateral: string,
	maxContractDuration: number,
	remainingStorage: number,
	totalStorage: number,
	prices: HostPrices
}

export type HostScan = {
	timestamp: string,
	success: boolean,
	latency: number,
	error: string,
	publicKey: string,
	network: string,
	node: string
}

export type HostBenchmark = {
	timestamp: string,
	success: boolean,
	error: string,
	uploadSpeed: number,
	downloadSpeed: number,
	ttfb: number,
	publicKey: string,
	network: string,
	node: string
}

export type HostScore = {
	prices: number,
	storage: number,
	collateral: number,
	interactions: number,
	uptime: number,
	age: number,
	version: number,
	latency: number,
	benchmarks: number,
	contracts: number,
	total: number
}

export type HostInteractions = {
	uptime: number,
	downtime: number,
	scanHistory: HostScan[],
	benchmarkHistory: HostBenchmark[],
	lastSeen: string,
	activeHosts: number,
	score: HostScore,
	successes: number,
	failures: number
}

export type Host = {
	id: number,
	rank: number,
	publicKey: string,
	firstSeen: string,
	knownSince: number,
	netaddress: string,
	blocked: boolean,
	v2: boolean,
	interactions: { [node: string]: HostInteractions },
	ipNets: string[],
	lastIPChange: string,
	score: HostScore,
	v2Settings:  HostSettingsV2,
	siamuxAddresses: string[],
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

export type NetworkStatus = {
	height: number,
	balance: string
}

export type NodeStatus = {
	online: boolean,
	version: string,
	networks: { [network: string]: NetworkStatus },
}

export type HostCount = {
	total: number,
	online: number
}

export type PriceChange = {
	timestamp: string,
	remainingStorage: number,
	totalStorage: number,
	collateral: string,
	storagePrice: string,
	uploadPrice: string,
	downloadPrice: string
}

export type HostSortType = {
	sortBy: 'id' | 'rank' | 'total' | 'used' | 'storage' | 'upload' | 'download',
	order: 'asc' | 'desc'
}

export type NetworkAverages = {
	storagePrice: string,
	collateral: string,
	uploadPrice: string,
	downloadPrice: string,
	contractDuration: number,
	available: boolean
}
