import './Averages.css'
import { useState, useEffect } from 'react'
import { Tooltip } from '../'
import {
	NetworkAverages,
	NodeStatus,
	blocksToTime,
	getStatus,
	toSia,
	useLocations
} from '../../api'

type AveragesProps = {
	darkMode: boolean,
	averages: { [tier: string]: NetworkAverages }
}

const AveragesTooltip = () => (
	<div>
		The prices given here do not count for any redundancy.
		They are given from the hosts' perspective.
	</div>
)

const tiers = [
	{ key: 'tier1', label: 'Tier 1', caption: 'Top 10 hosts by score' },
	{ key: 'tier2', label: 'Tier 2', caption: 'Top 100 minus tier 1' },
	{ key: 'tier3', label: 'Tier 3', caption: 'All remaining hosts' }
]

export const Averages = (props: AveragesProps) => {
	const [tier, setTier] = useState('tier1')
	const locations = useLocations()
	const [nodes, setNodes] = useState<{ [node: string]: NodeStatus }>()
	const [time, setTime] = useState(new Date())
	useEffect((): any => {
		const interval = setInterval(() => {
			setTime(new Date())
		}, 600000)
		return () => clearInterval(interval)
	}, [])
	useEffect(() => {
		let canceled = false
		let timer: ReturnType<typeof setTimeout>
		// The status request can get rate-limited during the initial burst
		// of API calls; retry with a backoff and keep the last known state.
		const fetchStatus = (attempt: number) => {
			getStatus()
			.then(data => {
				if (canceled) return
				if (data && data.nodes) {
					setNodes(data.nodes)
				} else if (attempt < 3) {
					timer = setTimeout(() => fetchStatus(attempt + 1), 5000 * (attempt + 1))
				}
			})
		}
		fetchStatus(0)
		return () => {
			canceled = true
			clearTimeout(timer)
		}
	}, [time])
	const available = tiers.filter(t =>
		props.averages[t.key] && props.averages[t.key].available === true)
	const selected = available.find(t => t.key === tier) || available[0]
	const data = selected ? props.averages[selected.key] : undefined
	return (
		<div className="averages-container">
			<div className="panel-h">
				<h2>Network averages</h2>
				<Tooltip className="averages-tooltip" darkMode={props.darkMode}>
					<AveragesTooltip/>
				</Tooltip>
			</div>
			<div className="averages-body">
				{available.length > 0 && selected && data ?
					<>
						<div className="averages-tiers" role="group" aria-label="Averages tier">
							{available.map(t => (
								<button
									key={t.key}
									tabIndex={1}
									aria-pressed={t.key === selected.key}
									onClick={() => setTier(t.key)}
								>{t.label}</button>
							))}
						</div>
						<div className="averages-caption">{selected.caption}</div>
						<div className="kv">
							<span className="kv-key">Storage price</span>
							<span className="kv-value">{toSia(data.storagePrice, true) + ' /TB·mo'}</span>
						</div>
						<div className="kv">
							<span className="kv-key">Collateral</span>
							<span className="kv-value">{toSia(data.collateral, true) + ' /TB·mo'}</span>
						</div>
						<div className="kv">
							<span className="kv-key">Ingress price</span>
							<span className="kv-value">{toSia(data.uploadPrice, false) + ' /TB'}</span>
						</div>
						<div className="kv">
							<span className="kv-key">Egress price</span>
							<span className="kv-value">{toSia(data.downloadPrice, false) + ' /TB'}</span>
						</div>
						<div className="kv">
							<span className="kv-key">Contract duration</span>
							<span className="kv-value">{blocksToTime(data.contractDuration)}</span>
						</div>
					</>
				:
					<div className="averages-empty">No averages available</div>
				}
			</div>
			<div className="averages-nodes">
				<div className="eyebrow">Benchmark nodes</div>
				<div className="averages-nodes-row">
					{locations.map(location => {
						const node = nodes ? nodes[location.short] : undefined
						const status = node ? (node.online ? 'online' : 'offline') : 'status unknown'
						return (
							<div
								className="averages-node-name"
								key={location.short}
								title={status}
								aria-label={location.long + ': ' + status}
							>
								<span className={'dot ' + (node ? (node.online ? 'dot-good' : 'dot-crit') : 'dot-warn')}></span>
								{location.long}
							</div>
						)
					})}
				</div>
			</div>
		</div>
	)
}
