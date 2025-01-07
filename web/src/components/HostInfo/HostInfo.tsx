import './HostInfo.css'
import { useState, useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import {
	Host,
	HostScore,
	NetworkAverages,
	getFlagEmoji,
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

const StoragePriceTooltip = (props: TooltipProps) => (
	<div>
		<div>Average storage prices:</div>
		<div className="host-info-tooltip-row">
			<span>Tier 1:</span>
			<span>{toSia(props.averages['tier1'].storagePrice, true) + '/TB/month'}</span>
		</div>
		{props.averages['tier2'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 2:</span>
				<span>{toSia(props.averages['tier2'].storagePrice, true) + '/TB/month'}</span>
			</div>
		}
		{props.averages['tier3'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 3:</span>
				<span>{toSia(props.averages['tier3'].storagePrice, true) + '/TB/month'}</span>
			</div>
		}
	</div>
)

const CollateralTooltip = (props: TooltipProps) => (
	<div>
		<div>Average collateral rates:</div>
		<div className="host-info-tooltip-row">
			<span>Tier 1:</span>
			<span>{toSia(props.averages['tier1'].collateral, true) + '/TB/month'}</span>
		</div>
		{props.averages['tier2'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 2:</span>
				<span>{toSia(props.averages['tier2'].collateral, true) + '/TB/month'}</span>
			</div>
		}
		{props.averages['tier3'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 3:</span>
				<span>{toSia(props.averages['tier3'].collateral, true) + '/TB/month'}</span>
			</div>
		}
	</div>
)

const UploadPriceTooltip = (props: TooltipProps) => (
	<div>
		<div>Average upload prices:</div>
		<div className="host-info-tooltip-row">
			<span>Tier 1:</span>
			<span>{toSia(props.averages['tier1'].uploadPrice, false) + '/TB'}</span>
		</div>
		{props.averages['tier2'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 2:</span>
				<span>{toSia(props.averages['tier2'].uploadPrice, false) + '/TB'}</span>
			</div>
		}
		{props.averages['tier3'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 3:</span>
				<span>{toSia(props.averages['tier3'].uploadPrice, false) + '/TB'}</span>
			</div>
		}
	</div>
)

const DownloadPriceTooltip = (props: TooltipProps) => (
	<div>
		<div>Average download prices:</div>
		<div className="host-info-tooltip-row">
			<span>Tier 1:</span>
			<span>{toSia(props.averages['tier1'].downloadPrice, false) + '/TB'}</span>
		</div>
		{props.averages['tier2'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 2:</span>
				<span>{toSia(props.averages['tier2'].downloadPrice, false) + '/TB'}</span>
			</div>
		}
		{props.averages['tier3'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 3:</span>
				<span>{toSia(props.averages['tier3'].downloadPrice, false) + '/TB'}</span>
			</div>
		}
	</div>
)

const ContractDurationTooltip = (props: TooltipProps) => (
	<div>
		<div>Average contract durations:</div>
		<div className="host-info-tooltip-row">
			<span>Tier 1:</span>
			<span>{blocksToTime(props.averages['tier1'].contractDuration)}</span>
		</div>
		{props.averages['tier2'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 2:</span>
				<span>{blocksToTime(props.averages['tier2'].contractDuration)}</span>
			</div>
		}
		{props.averages['tier3'].available === true &&
			<div className="host-info-tooltip-row">
				<span>Tier 3:</span>
				<span>{blocksToTime(props.averages['tier3'].contractDuration)}</span>
			</div>
		}
	</div>
)

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
	const [scoreExpanded, toggleScore] = useState(false)
	useEffect(() => {
		let network = (location.pathname.indexOf('/zen') === 0) ? 'zen' : 'mainnet'
		getAverages(network)
		.then(data => {
			if (data && data.averages) {
				setAverages(data.averages)
			}
		})
	}, [location, setAverages])
	return (
		<div className={'host-info-container' + (props.darkMode ? ' host-info-dark' : '')}>
			<table>
				<tbody>
					<tr><td>ID</td><td>{props.host.id}</td></tr>
					<tr><td>Rank</td><td>{props.host.rank}</td></tr>
					<tr><td>Public Key</td><td className="host-info-small">{props.host.publicKey}</td></tr>
					<tr><td>Address</td><td>{props.host.netaddress}</td></tr>
					<tr><td>Location</td><td>{getFlagEmoji(props.host.country)}</td></tr>
					<tr><td>Online</td><td>{online ? 'Yes' : 'No'}</td></tr>
					<tr><td>First Seen</td><td>{new Date(props.host.firstSeen).toDateString()}</td></tr>
					<tr><td>Last Seen</td><td>{lastSeen}</td></tr>
					<tr><td>Uptime</td><td>{uptime}</td></tr>
					<tr><td>Version</td><td>{props.host.settings.version === '' ? 'N/A' : props.host.settings.version}</td></tr>
					<tr><td>Release</td><td>{props.host.settings.release === '' ? 'N/A' : props.host.settings.release}</td></tr>
					<tr><td>Accepting Contracts</td><td>{props.host.settings.acceptingcontracts ? 'Yes' : 'No'}</td></tr>
					<tr>
						<td>Max Contract Duration</td>
						<td>
							{props.host.settings.maxduration === 0 ? 'N/A' : blocksToTime(props.host.settings.maxduration)}
							{averages['tier1'] &&
								<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
									<ContractDurationTooltip averages={averages}/>
								</Tooltip>
							}
						</td>
					</tr>
					<tr><td>Contract Price</td><td>{convertPrice(props.host.settings.contractprice)}</td></tr>
					<tr>
						<td>Storage Price</td>
						<td>
							{convertPricePerBlock(props.host.settings.storageprice)}
							{averages['tier1'] &&
								<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
									<StoragePriceTooltip averages={averages}/>
								</Tooltip>
							}
						</td>
					</tr>
					<tr>
						<td>Collateral</td>
						<td>
							{convertPricePerBlock(props.host.settings.collateral)}
							{averages['tier1'] &&
								<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
									<CollateralTooltip averages={averages}/>
								</Tooltip>
							}
						</td>
					</tr>
					<tr>
						<td>Upload Price</td>
						<td>
							{props.host.settings.uploadbandwidthprice === '0' ? '0 H/TB' : convertPrice(props.host.settings.uploadbandwidthprice + '0'.repeat(12)) + '/TB'}
							{averages['tier1'] &&
								<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
									<UploadPriceTooltip averages={averages}/>
								</Tooltip>
							}
						</td>
					</tr>
					<tr>
						<td>Download Price</td>
						<td>
							{props.host.settings.downloadbandwidthprice === '0' ? '0 H/TB' : convertPrice(props.host.settings.downloadbandwidthprice + '0'.repeat(12)) + '/TB'}
							{averages['tier1'] &&
								<Tooltip className="host-info-tooltip" darkMode={props.darkMode}>
									<DownloadPriceTooltip averages={averages}/>
								</Tooltip>
							}
						</td>
					</tr>
					<tr><td>Total Storage</td><td>{convertSize(props.host.settings.totalstorage)}</td></tr>
					<tr><td>Remaining Storage</td><td>{convertSize(props.host.settings.remainingstorage)}</td></tr>
					<tr><td>Active Hosts in Subnet</td><td>{activeHosts}</td></tr>
					<tr>
						<td className={'host-info-score' + (scoreExpanded ? ' host-info-score-expanded' : '')}>
							<span onClick={() => {toggleScore(!scoreExpanded)}}>
								Relative Score
							</span>
						</td>
						<td>{score.total.toPrecision(2)}</td>
					</tr>
					{scoreExpanded &&
						<tr className="host-info-score-details">
							<td>
								Accepting Contracts<br/>
								Prices<br/>
								Storage<br/>
								Collateral<br/>
								Interactions<br/>
								Uptime<br/>
								Age<br/>
								Version<br/>
								Latency<br/>
								Benchmarks
							</td>
							<td>
								{score.contracts.toPrecision(2)}<br/>
								{score.prices.toPrecision(2)}<br/>
								{score.storage.toPrecision(2)}<br/>
								{score.collateral.toPrecision(2)}<br/>
								{score.interactions.toPrecision(2)}<br/>
								{score.uptime.toPrecision(2)}<br/>
								{score.age.toPrecision(2)}<br/>
								{score.version.toPrecision(2)}<br/>
								{score.latency.toPrecision(2)}<br/>
								{score.benchmarks.toPrecision(2)}
							</td>
						</tr>
					}
				</tbody>
			</table>
		</div>
	)
}
