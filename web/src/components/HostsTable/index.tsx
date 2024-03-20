import './index.css'
import { Link } from 'react-router-dom'
import { Host, stripePrefix, useLocations } from '../../api'
import Tooltip from '../Tooltip'

type HostsTableProps = {
	darkMode: boolean,
	hosts: Host[]
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

const HostsTable = (props: HostsTableProps) => {
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
				if (!int) return
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
						<th style={{minWidth: '4rem'}}>ID</th>
						<th style={{minWidth: '20rem'}}>Net Address</th>
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
							<td>{host.id}</td>
							<td>
								<Link className="hosts-table-link" to={newLocation(host)}>
									{host.netaddress}
								</Link>
							</td>
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

export default HostsTable