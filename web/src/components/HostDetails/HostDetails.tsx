import './HostDetails.css'
import { useState, useEffect } from 'react'
import { useParams } from 'react-router'
import { useNavigate } from 'react-router-dom'
import {
	Host,
	PriceChange,
	getHost,
	getPriceChanges,
	stripePrefix,
	useLocations,
	countryByCode,
	countryFlag
} from '../../api'
import {
	HostInfo,
	HostMap,
	HostPrices,
	HostResults,
	Loader,
	NodeSelector
} from '../'

type HostDetailsProps = {
	darkMode: boolean,
	hosts: Host[]
}

export const HostDetails = (props: HostDetailsProps) => {
	const navigate = useNavigate()
	const { publicKey } = useParams()
	const [host, setHost] = useState<Host>()
	const [priceChanges, setPriceChanges] = useState<PriceChange[]>([])
	const [copied, setCopied] = useState(false)
	const { hosts } = props
	const network = window.location.pathname.toLowerCase().indexOf('zen') >= 0 ? 'zen' : 'mainnet'
	const [loadingHost, setLoadingHost] = useState(false)
	const [loadingPriceChanges, setLoadingPriceChanges] = useState(false)
	const [node, setNode] = useState('global')
	const locations = useLocations()
	useEffect(() => {
		let h = hosts.find(h => stripePrefix(h.publicKey) === publicKey)
		if (h) setHost(h)
		else {
			setLoadingHost(true)
			getHost(network, publicKey || '')
			.then(data => {
				if (data && data.host) {
					setHost(data.host)
				}
				setLoadingHost(false)
			})
		}
	}, [network, hosts, publicKey])
	useEffect(() => {
		if (!publicKey) return
		setLoadingPriceChanges(true)
		let from = new Date()
		from.setFullYear(from.getFullYear() - 1)
		getPriceChanges(network, publicKey, from)
		.then(data => {
			if (data && data.changes) {
				setPriceChanges(data.changes)
			}
			setLoadingPriceChanges(false)
		})
	}, [network, publicKey])
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
			return <span className="pill pill-ok"><span className="dot dot-good"></span>Accepting contracts</span>
		case 'medium':
			return <span className="pill pill-warn"><span className="dot dot-warn"></span>Not accepting contracts</span>
		default:
			return <span className="pill pill-bad"><span className="dot dot-crit"></span>Offline</span>
		}
	}
	const copyPublicKey = (host: Host) => {
		if (!navigator.clipboard) return
		navigator.clipboard.writeText(host.publicKey)
		.then(() => {
			setCopied(true)
			setTimeout(() => setCopied(false), 1500)
		})
	}
	const hostLocation = (host: Host): string => {
		const country = countryByCode(host.country) || ''
		return host.city ? host.city + ', ' + country : country
	}
	return (
		<div className="host-details-container">
			<button
				className="button-container host-details-back"
				tabIndex={1}
				onClick={() => {navigate(network === 'zen' ? '/zen' : '/')}}
			>← All hosts</button>
			{loadingHost || loadingPriceChanges ?
				<Loader
					darkMode={props.darkMode}
					className="host-details-loader"
				/>
			: (host ?
				<>
					<div className="panel host-details-id">
						{statusPill(host)}
						<div className="host-details-id-main">
							<h2 className="host-details-addr">{host.netaddress}</h2>
							<div className="host-details-pk">
								<code>{host.publicKey}</code>
								<button
									className="copy-btn"
									tabIndex={1}
									onClick={() => copyPublicKey(host)}
								>{copied ? 'COPIED' : 'COPY'}</button>
							</div>
						</div>
						<div className="host-details-chips">
							{hostLocation(host) !== '' &&
								<span className="host-details-chip">
									{hostLocation(host)}
									{host.country !== '' &&
										<span className="host-details-flag">{countryFlag(host.country)}</span>
									}
								</span>
							}
							{host.v2 === true && host.v2Settings.release !== '' &&
								<span className="host-details-chip"><b>{host.v2Settings.release}</b></span>
							}
							<span className="host-details-chip">
								first seen <b>{new Date(host.firstSeen).toLocaleDateString()}</b>
							</span>
						</div>
					</div>
					<div className="host-details-node">
						<NodeSelector
							darkMode={props.darkMode}
							node={node}
							setNode={setNode}
						/>
					</div>
					<div className="host-details-grid">
						<HostInfo
							darkMode={props.darkMode}
							host={host}
							node={node}
						/>
						<div className="host-details-side">
							<div className="panel host-details-map-panel">
								<div className="panel-h">
									<h2>Location</h2>
								</div>
								<div className="host-details-map-body">
									<HostMap
										darkMode={props.darkMode}
										network={network}
										host={host}
									/>
								</div>
							</div>
							<HostResults
								darkMode={props.darkMode}
								host={host}
								node={node}
							/>
						</div>
					</div>
					<HostPrices
						darkMode={props.darkMode}
						data={priceChanges}
					/>
				</>
			:
				<div className="host-not-found">Host not found</div>
			)}
		</div>
	)
}
