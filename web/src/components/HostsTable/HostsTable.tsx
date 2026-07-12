import './HostsTable.css'
import { useNavigate } from 'react-router-dom'
import {
	Host,
	HostSortType,
	stripePrefix,
	useLocations,
	convertSize,
	countryByCode,
	countryFlag,
	toSia
} from '../../api'
import { Sort, Tooltip } from '../'

type HostsTableProps = {
	darkMode: boolean,
	hosts: Host[],
	sorting: HostSortType,
	changeSorting: (sorting: HostSortType) => any
}

const StatusTooltip = () => (
	<div className="hosts-table-legend">
		<div className="hosts-table-legend-row">
			<span className="pill pill-ok"><span className="dot dot-good"></span>Accepting</span>
			online, accepting contracts
		</div>
		<div className="hosts-table-legend-row">
			<span className="pill pill-warn"><span className="dot dot-warn"></span>Not accepting</span>
			online, not accepting contracts
		</div>
		<div className="hosts-table-legend-row">
			<span className="pill pill-bad"><span className="dot dot-crit"></span>Offline</span>
			failed the last scans
		</div>
	</div>
)

export const HostsTable = (props: HostsTableProps) => {
	const navigate = useNavigate()
	const newLocation = (host: Host) => {
		let path = window.location.pathname
		if (path[path.length - 1] !== '/') path += '/'
		return path + 'host/' + stripePrefix(host.publicKey)
	}
	const locations = useLocations()
	const hostStatus = (host: Host) => {
		let online = false
		if (host.interactions) {
			locations.forEach(location => {
				let int = host.interactions[location.short]
				if (!int || !int.scanHistory) return
				if (int.scanHistory.length > 0 && int.scanHistory[0].success === true &&
					((int.scanHistory.length > 1 && int.scanHistory[1].success === true) ||
					int.scanHistory.length === 1)) {
					online = true
				}
			})
		}
		if (!online || host.v2 === false) return 'bad'
		if (host.v2 === true && host.v2Settings.acceptingContracts === false) return 'medium'
		return 'good'
	}
	const statusPill = (host: Host) => {
		switch (hostStatus(host)) {
		case 'good':
			return <span className="pill pill-ok"><span className="dot dot-good"></span>Accepting</span>
		case 'medium':
			return <span className="pill pill-warn"><span className="dot dot-warn"></span>Not accepting</span>
		default:
			return <span className="pill pill-bad"><span className="dot dot-crit"></span>Offline</span>
		}
	}
	const getStoragePrice = (host: Host): string => (host.v2 === true ? toSia(host.v2Settings.prices.storagePrice, true) : 'N/A')
	const getIngressPrice = (host: Host): string => (host.v2 === true ? toSia(host.v2Settings.prices.ingressPrice, false) : 'N/A')
	const getEgressPrice = (host: Host): string => (host.v2 === true ? toSia(host.v2Settings.prices.egressPrice, false) : 'N/A')
	const getTotalStorage = (host: Host): number => (host.v2 === true ? host.v2Settings.totalStorage * 4 * 1024 * 1024 : 0)
	const getUsedStorage = (host: Host): number => (host.v2 === true ? (host.v2Settings.totalStorage - host.v2Settings.remainingStorage) * 4 * 1024 * 1024 : 0)
	const utilization = (host: Host): number => {
		const total = getTotalStorage(host)
		return total > 0 ? Math.round(getUsedStorage(host) * 100 / total) : 0
	}
	const hostLocation = (host: Host): string => {
		const country = countryByCode(host.country) || ''
		return host.city ? host.city + ', ' + country : country
	}
	return (
		<div className="hosts-table-container">
			<table>
				<thead>
					<tr>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'rank' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'rank', order: order })
								}}
							>Rank</Sort>
						</th>
						<th>Host</th>
						<th className="hosts-table-num">
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'storage' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'storage', order: order })
								}}
							>Storage /TB·mo</Sort>
						</th>
						<th className="hosts-table-num">
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'upload' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'upload', order: order })
								}}
							>Ingress /TB</Sort>
						</th>
						<th className="hosts-table-num">
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'download' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'download', order: order })
								}}
							>Egress /TB</Sort>
						</th>
						<th className="hosts-table-num">
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'used' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'used', order: order })
								}}
							>Used</Sort>
						</th>
						<th className="hosts-table-num">
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'total' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'total', order: order })
								}}
							>Total</Sort>
						</th>
						<th>
							<div className="hosts-table-status-header">
								Status
								<Tooltip darkMode={props.darkMode}><StatusTooltip/></Tooltip>
							</div>
						</th>
					</tr>
				</thead>
				<tbody>
					{props.hosts.map(host => (
						<tr
							key={host.publicKey}
							tabIndex={1}
							onClick={() => navigate(newLocation(host))}
							onKeyUp={(event: React.KeyboardEvent<HTMLTableRowElement>) => {
								if (event.key === 'Enter') navigate(newLocation(host))
							}}
						>
							<td className="hosts-table-rank">#{host.rank}</td>
							<td>
								<div className="hosts-table-addr">{host.netaddress}</div>
								<div className="hosts-table-loc">
									{hostLocation(host)}
									{host.country !== '' &&
										<span className="hosts-table-flag">{countryFlag(host.country)}</span>
									}
								</div>
							</td>
							<td className="hosts-table-num">{getStoragePrice(host)}</td>
							<td className="hosts-table-num">{getIngressPrice(host)}</td>
							<td className="hosts-table-num">{getEgressPrice(host)}</td>
							<td className="hosts-table-num">
								<div
									className="hosts-table-used"
									title={utilization(host) + '% of advertised storage in use'}
								>
									<span className="meter">
										<i style={{width: utilization(host) + '%'}}></i>
									</span>
									<span>{convertSize(getUsedStorage(host))}</span>
								</div>
							</td>
							<td className="hosts-table-num">{convertSize(getTotalStorage(host))}</td>
							<td>{statusPill(host)}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	)
}
