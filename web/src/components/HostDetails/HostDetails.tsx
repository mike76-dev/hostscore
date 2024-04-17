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
    useLocations
} from '../../api'
import {
    Button,
    HostInfo,
    HostMap,
    HostPrices,
    HostResults,
    Loader,
    NodeSelector
} from '../'
import Back from '../../assets/back.png'

type HostDetailsProps = {
	darkMode: boolean,
	hosts: Host[]
}

export const HostDetails = (props: HostDetailsProps) => {
    const locations = useLocations()
    const navigate = useNavigate()
	const { publicKey } = useParams()
	const [host, setHost] = useState<Host>()
    const [priceChanges, setPriceChanges] = useState<PriceChange[]>([])
	const { hosts } = props
	const network = (window.location.pathname.toLowerCase().indexOf('zen') >= 0 ? 'zen' : 'mainnet')
	const [loadingHost, setLoadingHost] = useState(false)
    const [loadingPriceChanges, setLoadingPriceChanges] = useState(false)
    const nodes = ['global'].concat(locations)
    const [node, setNode] = useState(nodes[0])
	useEffect(() => {
		let h = hosts.find(h => stripePrefix(h.publicKey) === publicKey)
		if (h) setHost(h)
		else {
			setLoadingHost(true)
			getHost(network, publicKey || '')
			.then(data => {
				if (data && data.status === 'ok' && data.host) {
					setHost(data.host)
				}
				setLoadingHost(false)
			})
		}
	}, [network, hosts, publicKey])
    useEffect(() => {
        setLoadingPriceChanges(true)
        getPriceChanges(network, publicKey || '')
        .then(data => {
            if (data && data.status === 'ok' && data.priceChanges) {
                setPriceChanges(data.priceChanges)
            }
            setLoadingPriceChanges(false)
        })
    }, [network, publicKey, locations.length])
	return (
		<div className={'host-details-container' + (props.darkMode ? ' host-details-dark' : '')}>
			{loadingHost || loadingPriceChanges ?
				<Loader
                    darkMode={props.darkMode}
                    className="host-details-loader"
                />
			: (host ?
                <>
                    <div className="host-details-subcontainer">
                        <NodeSelector
                            darkMode={props.darkMode}
                            nodes={nodes}
                            node={node}
                            setNode={setNode}
                        />
        				<HostInfo
	        				darkMode={props.darkMode}
		        			host={host}
                            node={node}
				        />
                        <HostMap
                            darkMode={props.darkMode}
                            network={network}
                            host={host}
                        />
                        <HostPrices
                            darkMode={props.darkMode}
                            data={priceChanges}
                        />
                    </div>
                    <HostResults
                        darkMode={props.darkMode}
                        host={host}
                        node={node}
                    />
                </>
			:
				<div className="host-not-found">Host Not Found</div>
			)}
            <Button
				icon={Back}
				caption="home"
				darkMode={props.darkMode}
				onClick={() => {navigate(network === 'mainnet' ? '/' : '/zen/')}}
			/>
		</div>
	)
}
