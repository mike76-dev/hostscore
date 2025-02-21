import './HostsTable.css'
import { Link } from 'react-router-dom'
import {
	Host,
	HostSortType,
	stripePrefix,
	useLocations,
	convertSize,
	countryByCode,
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
	<div className="hosts-table-tooltip">
		<div className="hosts-table-flex">
			<div className="hosts-table-status hosts-table-status-good"></div>
			<div className="hosts-table-tooltip-text">host is online</div>
		</div>
		<div className="hosts-table-flex">
			<div className="hosts-table-status hosts-table-status-medium"></div>
			<div className="hosts-table-tooltip-text">host is not accepting contracts</div>
		</div>
		<div className="hosts-table-flex">
			<div className="hosts-table-status hosts-table-status-bad"></div>
			<div className="hosts-table-tooltip-text">host is offline</div>
		</div>
	</div>
)

export const HostsTable = (props: HostsTableProps) => {
	const newLocation = (host: Host) => {
		let href = window.location.href
		if (href[href.length - 1] === '/') {
			return href + 'host/' + stripePrefix(host.publicKey)
		}
		return href + '/host/' + stripePrefix(host.publicKey)
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
		if (!online) return 'bad'
		if ((host.v2 === true && host.v2Settings.acceptingContracts === false) || (host.v2 === false && host.settings.acceptingcontracts === false)) return 'medium'
		return 'good'
	}
	const getStoragePrice = (host: Host): string => {
		let sp = (host.v2 === true ? host.v2Settings.prices.storagePrice : host.settings.storageprice)
		return toSia(sp, true) + '/TB/month'
	}
	const getIngressPrice = (host: Host): string => {
		let ip = (host.v2 === true ? host.v2Settings.prices.ingressPrice : host.settings.uploadbandwidthprice)
		return toSia(ip, false) + '/TB'
	}
	const getEgressPrice = (host: Host): string => {
		let ep = (host.v2 === true ? host.v2Settings.prices.egressPrice : host.settings.downloadbandwidthprice)
		return toSia(ep, false) + '/TB'
	}
	const getTotalStorage = (host: Host): number => (host.v2 === true ? host.v2Settings.totalStorage : host.settings.totalstorage)
	const getRemainingStorage = (host: Host): number => (host.v2 === true ? host.v2Settings.remainingStorage : host.settings.remainingstorage)
	const getAddress = (host: Host) => (host.v2 === true ? (host.siamuxAddresses[0] || '') : host.settings.netaddress)
	return (
		<div className={'hosts-table-container' + (props.darkMode ? ' hosts-table-dark' : '')}>
			<table>
				<thead>
					<tr>
						<th style={{minWidth: '4rem'}}>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'rank' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'rank', order: order })
								}}
							>Rank</Sort>
						</th>
						<th style={{minWidth: '4rem'}}>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'id' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'id', order: order })
								}}
							>ID</Sort>
						</th>
						<th style={{minWidth: '20rem'}}>Net Address</th>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'storage' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'storage', order: order })
								}}
							>Storage Price</Sort>
						</th>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'upload' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'upload', order: order })
								}}
							>Upload Price</Sort>
						</th>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'download' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'download', order: order })
								}}
							>Download Price</Sort>
						</th>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'used' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'used', order: order })
								}}
							>Used Storage</Sort>
						</th>
						<th>
							<Sort
								darkMode={props.darkMode}
								order={props.sorting.sortBy === 'total' ? props.sorting.order : 'none'}
								setOrder={(order: 'asc' | 'desc') => {
									props.changeSorting({ sortBy: 'total', order: order })
								}}
							>Total Storage</Sort>
						</th>
						<th>Country</th>
						<th>
							<div className="hosts-table-flex">
								Status
								<Tooltip darkMode={props.darkMode}><StatusTooltip/></Tooltip>
							</div>
						</th>
					</tr>
				</thead>
				<tbody>
					{props.hosts.map(host => (
						<tr key={host.publicKey}>
							<td>{host.rank}</td>
							<td>{host.id}</td>
							<td>
								<Link
									className="hosts-table-link"
									to={newLocation(host)}
									tabIndex={1}
								>
									{getAddress(host)}
								</Link>
							</td>
							<td style={{textAlign: 'center'}}>{getStoragePrice(host)}</td>
							<td style={{textAlign: 'center'}}>{getIngressPrice(host)}</td>
							<td style={{textAlign: 'center'}}>{getEgressPrice(host)}</td>
							<td style={{textAlign: 'center'}}>{convertSize(getTotalStorage(host) - getRemainingStorage(host))}</td>
							<td style={{textAlign: 'center'}}>{convertSize(getTotalStorage(host))}</td>
							<td>{countryByCode(host.country)}</td>
							<td>
								<div className={'hosts-table-status hosts-table-status-' + hostStatus(host)}></div>
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	)
}
