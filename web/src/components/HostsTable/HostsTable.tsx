import './HostsTable.css'
import { Link } from 'react-router-dom'
import {
    Host,
    HostSortType,
    stripePrefix,
    useLocations,
    convertSize,
    countryByCode,
    convertPriceRaw
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
				let int = host.interactions[location]
				if (!int || !int.scanHistory) return
				if (int.scanHistory.length > 0 && int.scanHistory[0].success === true &&
					((int.scanHistory.length > 1 && int.scanHistory[1].success === true) ||
					int.scanHistory.length === 1)) {
					online = true
				}
			})
		}
		if (!online) return 'bad'
		if (host.settings.acceptingcontracts === false) return 'medium'
		return 'good'
	}
    const toSia = (value: string, perBlock: boolean) => {
        let price = convertPriceRaw(value)
        if (perBlock) price *= 144 * 30
        if (price < 1e-12) return '0 H'
        if (price < 1e-9) return (price * 1000).toFixed(0) + ' pS'
        if (price < 1e-6) return (price * 1000).toFixed(0) + ' nS'
        if (price < 1e-3) return (price * 1000).toFixed(0) + ' uS'
        if (price < 1) return (price * 1000).toFixed(0) + ' mS'
        if (price < 10) return price.toFixed(1) + ' SC'
        if (price < 1e3) return price.toFixed(0) + ' SC'
        if (price < 1e4) return (price / 1000).toFixed(1) + ' KS'
        return (price / 1000).toFixed(0) + ' KS'
    }
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
                        <th>Storage Price</th>
                        <th>Upload Price</th>
                        <th>Download Price</th>
                        <th>
                            <Sort
                                darkMode={props.darkMode}
                                order={props.sorting.sortBy === 'remaining' ? props.sorting.order : 'none'}
                                setOrder={(order: 'asc' | 'desc') => {
                                    props.changeSorting({ sortBy: 'remaining', order: order })
                                }}
                            >Remaining Storage</Sort>
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
									{host.netaddress}
								</Link>
							</td>
                            <td style={{textAlign: 'center'}}>{toSia(host.settings.storageprice, true) + '/TB/month'}</td>
                            <td style={{textAlign: 'center'}}>{toSia(host.settings.uploadbandwidthprice, false) + '/TB'}</td>
                            <td style={{textAlign: 'center'}}>{toSia(host.settings.downloadbandwidthprice, false) + '/TB'}</td>
                            <td style={{textAlign: 'center'}}>{convertSize(host.settings.remainingstorage)}</td>
                            <td style={{textAlign: 'center'}}>{convertSize(host.settings.totalstorage)}</td>
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
