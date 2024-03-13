import './index.css'
import {
	Host,
	getFlagEmoji,
	blocksToTime,
	convertSize,
	convertPrice,
	convertPricePerBlock
} from '../../api'

type HostInfoProps = {
	darkMode: boolean,
	host: Host
}

const HostInfo = (props: HostInfoProps) => {
	return (
		<div className={'host-info-container' + (props.darkMode ? ' host-info-dark' : '')}>
			<table>
				<tbody>
				<tr><td>ID</td><td>{props.host.id}</td></tr>
				<tr><td>Public Key</td><td className="host-info-small">{props.host.publicKey}</td></tr>
				<tr><td>Address</td><td>{props.host.netaddress}</td></tr>
				<tr><td>Location</td><td>{getFlagEmoji(props.host.country)}</td></tr>
				<tr><td>First Seen</td><td>{new Date(props.host.firstSeen).toDateString()}</td></tr>
				<tr><td>Last Seen</td><td>{props.host.lastSeen === '0001-01-01T00:00:00Z' ? 'N/A' : new Date(props.host.lastSeen).toDateString()}</td></tr>
				<tr><td>Uptime</td><td>{props.host.downtime + props.host.uptime === 0 ? '0%' : (props.host.uptime * 100 / (props.host.uptime + props.host.downtime)).toFixed(1) + '%'}</td></tr>
				<tr><td>Version</td><td>{props.host.settings.version === '' ? 'N/A' : props.host.settings.version}</td></tr>
				<tr><td>Accepting Contracts</td><td>{props.host.settings.acceptingcontracts ? 'Yes' : 'No'}</td></tr>
				<tr><td>Max Contract Duration</td><td>{props.host.settings.maxduration === 0 ? 'N/A' : blocksToTime(props.host.settings.maxduration)}</td></tr>
				<tr><td>Contract Price</td><td>{convertPrice(props.host.settings.contractprice)}</td></tr>
				<tr><td>Storage Price</td><td>{convertPricePerBlock(props.host.settings.storageprice)}</td></tr>
				<tr><td>Collateral</td><td>{convertPricePerBlock(props.host.settings.collateral)}</td></tr>
				<tr><td>Upload Price</td><td>{props.host.settings.uploadbandwidthprice === '0' ? '0 H/TB' : convertPrice(props.host.settings.uploadbandwidthprice + '0'.repeat(12)) + '/TB'}</td></tr>
				<tr><td>Download Price</td><td>{props.host.settings.downloadbandwidthprice === '0' ? '0 H/TB' : convertPrice(props.host.settings.downloadbandwidthprice + '0'.repeat(12)) + '/TB'}</td></tr>
				<tr><td>Total Storage</td><td>{convertSize(props.host.settings.totalstorage)}</td></tr>
				<tr><td>Remaining Storage</td><td>{convertSize(props.host.settings.remainingstorage)}</td></tr>
				<tr><td>Active Hosts in Subnet</td><td>{props.host.activeHosts}</td></tr>
				</tbody>
			</table>
		</div>
	)
}

export default HostInfo