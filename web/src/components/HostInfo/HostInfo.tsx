import './HostInfo.css'
import { useState } from 'react'
import {
	Host,
    HostScore,
	getFlagEmoji,
	blocksToTime,
	convertSize,
	convertPrice,
	convertPricePerBlock,
	useLocations
} from '../../api'

type HostInfoProps = {
	darkMode: boolean,
	host: Host,
	node: string,
}

type Interactions = {
	lastSeen: string,
	uptime: string,
	activeHosts: number,
    score: HostScore,
}

export const HostInfo = (props: HostInfoProps) => {
	const locations = useLocations()
	const interactions = (): Interactions => {
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
            total: 0
        }
		if (props.node === 'global') {
			locations.forEach(location => {
				let int = props.host.interactions[location]
				if (!int) return
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
				ls = new Date(int.lastSeen)
				ut = int.uptime
				dt = int.downtime
				activeHosts = int.activeHosts
                score = int.score
			}
		}
		let lastSeen = (ls.getFullYear() <= 1970) ? 'N/A' : ls.toDateString()
		let uptime = dt + ut === 0 ? '0%' : (ut * 100 / (ut + dt)).toFixed(1) + '%'
		return { lastSeen, uptime, activeHosts, score }
	}
	const { lastSeen, uptime, activeHosts, score } = interactions()
    const [scoreExpanded, toggleScore] = useState(false)
	return (
		<div className={'host-info-container' + (props.darkMode ? ' host-info-dark' : '')}>
			<table>
				<tbody>
    				<tr><td>ID</td><td>{props.host.id}</td></tr>
                    <tr><td>Rank</td><td>{props.host.rank}</td></tr>
	    			<tr><td>Public Key</td><td className="host-info-small">{props.host.publicKey}</td></tr>
		    		<tr><td>Address</td><td>{props.host.netaddress}</td></tr>
			    	<tr><td>Location</td><td>{getFlagEmoji(props.host.country)}</td></tr>
				    <tr><td>First Seen</td><td>{new Date(props.host.firstSeen).toDateString()}</td></tr>
    				<tr><td>Last Seen</td><td>{lastSeen}</td></tr>
	    			<tr><td>Uptime</td><td>{uptime}</td></tr>
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
