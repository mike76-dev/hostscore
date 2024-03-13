import './index.css'
import { useState, useEffect } from 'react'
import { useParams } from 'react-router'
import { Host, getHost, stripePrefix } from '../../api'
import HostInfo from '../HostInfo'
import Loader from '../Loader'

type HostDetailsProps = {
	darkMode: boolean,
	hosts: Host[]
}

const HostDetails = (props: HostDetailsProps) => {
	const { publicKey } = useParams()
	const [host, setHost] = useState<Host>()
	const { hosts } = props
	const network = (window.location.pathname.toLowerCase().indexOf('zen') >= 0 ? 'zen' : 'mainnet')
	const [loading, setLoading] = useState(false)
	useEffect(() => {
		let h = hosts.find(h => stripePrefix(h.publicKey) === publicKey)
		if (h) setHost(h)
		else {
			setLoading(true)
			getHost(network, publicKey || '')
			.then(data => {
				if (data && data.status === 'ok' && data.host) {
					setHost(data.host)
				}
				setLoading(false)
			})
		}
	}, [network, hosts, publicKey])
	return (
		<div className={'host-details-container' + (props.darkMode ? ' host-details-dark' : '')}>
			{loading ?
				<Loader darkMode={props.darkMode}/>
			: (host ?
				<HostInfo
					darkMode={props.darkMode}
					host={host}
				/>
			:
				<div className="host-not-found">Host Not Found</div>
			)}
		</div>
	)
}

export default HostDetails