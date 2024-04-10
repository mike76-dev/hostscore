import './HostsTable.css'
import { Link } from 'react-router-dom'
import {
    Host,
    HostSortType,
    stripePrefix,
    useLocations,
    convertSize,
    countryByCode
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
	return (
		<div className={'hosts-table-container' + (props.darkMode ? ' hosts-table-dark' : '')}>
			<table>
				<thead>
					<tr>
                        <th style={{minWidth: '4rem'}}>Rank
                            <Sort
                                darkMode={props.darkMode}
                                order={props.sorting.sortBy === 'rank' ? props.sorting.order : 'none'}
                                setOrder={(order: 'asc' | 'desc') => {
                                    props.changeSorting({ sortBy: 'rank', order: order })
                                }}
                            />
                        </th>
						<th style={{minWidth: '4rem'}}>ID
                            <Sort
                                darkMode={props.darkMode}
                                order={props.sorting.sortBy === 'id' ? props.sorting.order : 'none'}
                                setOrder={(order: 'asc' | 'desc') => {
                                    props.changeSorting({ sortBy: 'id', order: order })
                                }}
                            />
                        </th>
						<th style={{minWidth: '20rem'}}>Net Address</th>
                        <th>Remaining Storage
                            <Sort
                                darkMode={props.darkMode}
                                order={props.sorting.sortBy === 'remaining' ? props.sorting.order : 'none'}
                                setOrder={(order: 'asc' | 'desc') => {
                                    props.changeSorting({ sortBy: 'remaining', order: order })
                                }}
                            />
                        </th>
                        <th>Total Storage
                            <Sort
                                darkMode={props.darkMode}
                                order={props.sorting.sortBy === 'total' ? props.sorting.order : 'none'}
                                setOrder={(order: 'asc' | 'desc') => {
                                    props.changeSorting({ sortBy: 'total', order: order })
                                }}
                            />
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
