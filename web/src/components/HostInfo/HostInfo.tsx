import './HostInfo.css'
import { useState, useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import {
	Host,
	HostScore,
	NetworkAverages,
	blocksToTime,
	convertSize,
	convertPrice,
	convertPricePerBlock,
	toSia,
	useLocations,
	getAverages
} from '../../api'
import { Tooltip } from '../'

type HostInfoProps = {
	darkMode: boolean,
	host: Host,
	node: string,
}

type Interactions = {
	online: boolean,
	lastSeen: string,
	uptime: string,
	activeHosts: number,
	score: HostScore,
}

interface TooltipProps {
	averages: { [tier: string]: NetworkAverages }
}

const AveragesTooltip = (props: TooltipProps & {
	title: string,
	format: (data: NetworkAverages) => string
}) => (
	<div>
		<div>{props.title}</div>
		{['tier1', 'tier2', 'tier3'].map((tier, index) => (
			props.averages[tier] && props.averages[tier].available === true &&
				<div className="host-info-tooltip-row" key={tier}>
					<span>{'Tier ' + (index + 1) + ':'}</span>
					<span>{props.format(props.averages[tier])}</span>
				</div>
		))}
	</div>
)

const scoreComponents = (score: HostScore): { key: string, value: number }[] => ([
	{ key: 'accepting contracts', value: score.contracts },
	{ key: 'prices', value: score.prices },
	{ key: 'storage', value: score.storage },
	{ key: 'collateral', value: score.collateral },
	{ key: 'interactions', value: score.interactions },
	{ key: 'uptime', value: score.uptime },
	{ key: 'age', value: score.age },
	{ key: 'version', value: score.version },
	{ key: 'latency', value: score.latency },
	{ key: 'benchmarks', value: score.benchmarks }
])

export const HostInfo = (props: HostInfoProps) => {
	const locations = useLocations()
	const location = useLocation()
	const [averages, setAverages] = useState<{ [tier: string]: NetworkAverages }>({})
	const interactions = (): Interactions => {
		let online = false
		let ls = new Date('0001-01-01T00:00:00Z')
		let ut = 0
		let dt = 0
		let activeHosts = 0
		let score: HostScore = {
			prices: 0,
			storage: 0,
			collateral: 0,
			interactions: 0,
			uptime: 0,
			age: 0,
			version: 0,
			latency: 0,
			benchmarks: 0,
			contracts: 0,
			total: 0
		}
		if (props.node === 'global') {
			locations.forEach(location => {
				let int = props.host.interactions[location.short]
				if (!int || !int.scanHistory) return
				if (int.scanHistory.length > 0 && int.scanHistory[0].success === true &&
					((int.scanHistory.length > 1 && int.scanHistory[1].success === true) ||
					int.scanHistory.length === 1)) {
					online = true
				}
				if (int.lastSeen.indexOf('0001-01-01') < 0) {
					let nls = new Date(int.lastSeen)
					if (nls > ls) ls = nls
				}
				ut += int.uptime
				dt += int.downtime
				if (int.activeHosts > activeHosts) activeHosts = int.activeHosts
			})
			score = props.host.score
		} else {
			let int = props.host.interactions[props.node]
			if (int) {
				if (int.scanHistory.length > 0 && int.scanHistory[0].success === true &&
					((int.scanHistory.length > 1 && int.scanHistory[1].success === true) ||
					int.scanHistory.length === 1)) {
					online = true
				}
				ls = new Date(int.lastSeen)
				ut = int.uptime
				dt = int.downtime
				activeHosts = int.activeHosts
				score = int.score
			}
		}
		let lastSeen = (ls.getFullYear() <= 1970) ? 'N/A' : ls.toDateString()
		let uptime = dt + ut === 0 ? '0%' : (ut * 100 / (ut + dt)).toFixed(1) + '%'
		return { online, lastSeen, uptime, activeHosts, score }
	}
	const { online, lastSeen, uptime, activeHosts, score } = interactions()
	const getRelease = (host: Host): string => (host.v2 === true ? host.v2Settings.release : 'N/A')
	const isAcceptingContracts = (host: Host): string => (host.v2 === true ? (host.v2Settings.acceptingContracts ? 'Yes' : 'No') : 'No')
	const getMaxDuration = (host: Host): string => (host.v2 === true ? blocksToTime(host.v2Settings.maxContractDuration) : 'N/A')
	const getContractPrice = (host: Host): string => (host.v2 === true ? convertPrice(host.v2Settings.prices.contractPrice) : 'N/A')
	const getStoragePrice = (host: Host): string => (host.v2 === true ? convertPricePerBlock(host.v2Settings.prices.storagePrice) : 'N/A')
	const getCollateral = (host: Host): string => (host.v2 === true ? convertPricePerBlock(host.v2Settings.prices.collateral) : 'N/A')
	const getIngressPrice = (host: Host): string => {
		if (host.v2 === true) {
			let ip = host.v2Settings.prices.ingressPrice
			return ip === '0' ? '0 H/TB' : convertPrice(ip + '0'.repeat(12)) + '/TB'
		}
		return 'N/A'
	}
	const getEgressPrice = (host: Host): string => {
		if (host.v2 === true) {
			let ep = host.v2Settings.prices.egressPrice
			return ep === '0' ? '0 H/TB' : convertPrice(ep + '0'.repeat(12)) + '/TB'
		}
		return 'N/A'
	}
	const getTotalStorage = (host: Host): string => (host.v2 === true ? convertSize(host.v2Settings.totalStorage * 4 * 1024 * 1024) : 'N/A')
	const getRemainingStorage = (host: Host): string => (host.v2 === true ? convertSize(host.v2Settings.remainingStorage * 4 * 1024 * 1024) : 'N/A')
	useEffect(() => {
		let network = location.pathname.indexOf('/zen') === 0 ? 'zen' : 'mainnet'
		getAverages(network)
		.then(data => {
			if (data && data.averages) {
				setAverages(data.averages)
			}
		})
	}, [location, setAverages])
	const components = scoreComponents(score)
	const lowest = Math.min(...components.map(c => c.value))
	const row = (key: string, value: React.ReactNode) => (
		<div className="kv">
			<span className="kv-key">{key}</span>
			<span className="kv-value">{value}</span>
		</div>
	)
	return (
		<div className="panel host-info-container">
			<div className="panel-h">
				<h2>Overview</h2>
				<span className="panel-sub">
					{props.node === 'global' ? 'across all benchmark nodes' :
						'as seen from ' + (locations.find(l => l.short === props.node)?.long || props.node)}
				</span>
			</div>
			<div className="host-info-body">
				{row('ID', props.host.id)}
				{row('Rank', '#' + props.host.rank)}
				{row('Online', online ? 'Yes' : 'No')}
				{row('First seen', new Date(props.host.firstSeen).toDateString())}
				{row('Last seen', lastSeen)}
				{row('Uptime', uptime)}
				{row('Release', getRelease(props.host))}
				{row('Accepting contracts', isAcceptingContracts(props.host))}
				{row('Max contract duration', <>
					{getMaxDuration(props.host)}
					{averages['tier1'] &&
						<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
							<AveragesTooltip
								averages={averages}
								title="Average contract durations:"
								format={data => blocksToTime(data.contractDuration)}
							/>
						</Tooltip>
					}
				</>)}
				{row('Contract price', getContractPrice(props.host))}
				{row('Storage price', <>
					{getStoragePrice(props.host)}
					{averages['tier1'] &&
						<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
							<AveragesTooltip
								averages={averages}
								title="Average storage prices:"
								format={data => toSia(data.storagePrice, true) + '/TB/month'}
							/>
						</Tooltip>
					}
				</>)}
				{row('Collateral', <>
					{getCollateral(props.host)}
					{averages['tier1'] &&
						<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
							<AveragesTooltip
								averages={averages}
								title="Average collateral rates:"
								format={data => toSia(data.collateral, true) + '/TB/month'}
							/>
						</Tooltip>
					}
				</>)}
				{row('Ingress price', <>
					{getIngressPrice(props.host)}
					{averages['tier1'] &&
						<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
							<AveragesTooltip
								averages={averages}
								title="Average ingress prices:"
								format={data => toSia(data.uploadPrice, false) + '/TB'}
							/>
						</Tooltip>
					}
				</>)}
				{row('Egress price', <>
					{getEgressPrice(props.host)}
					{averages['tier1'] &&
						<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
							<AveragesTooltip
								averages={averages}
								title="Average egress prices:"
								format={data => toSia(data.downloadPrice, false) + '/TB'}
							/>
						</Tooltip>
					}
				</>)}
				{row('Total storage', getTotalStorage(props.host))}
				{row('Remaining storage', getRemainingStorage(props.host))}
				{row('Active hosts in subnet', activeHosts)}
			</div>
			<div className="host-info-score">
				<div className="panel-h">
					<h2>Score breakdown</h2>
					<span className="panel-sub">relative to the whole network</span>
				</div>
				<div className="host-info-score-body">
					<div className="host-info-score-total">
						<span className="host-info-score-value">{score.total.toPrecision(2)}</span>
						<span className="host-info-score-rank">RANK #{props.host.rank}</span>
					</div>
					{components.map(component => (
						<div
							className={'host-info-score-row' + (component.value === lowest ? ' host-info-score-low' : '')}
							key={component.key}
						>
							<span className="host-info-score-key">{component.key}</span>
							<span className="host-info-score-bar">
								<i style={{width: Math.max(1.5, component.value * 100) + '%'}}></i>
							</span>
							<span className="host-info-score-number">{component.value.toPrecision(2)}</span>
						</div>
					))}
					<div className="host-info-score-note">
						The total score is the product of all factors.
					</div>
				</div>
			</div>
		</div>
	)
}
