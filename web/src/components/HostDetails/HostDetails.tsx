import './HostDetails.css'
import { useState, useEffect } from 'react'
import { useParams } from 'react-router'
import { useNavigate } from 'react-router-dom'
import {
    Host,
    PriceChange,
    getHost,
    getPriceChanges,
    stripePrefix
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
    const navigate = useNavigate()
	const { publicKey } = useParams()
	const [host, setHost] = useState<Host>()
    const [priceChanges, setPriceChanges] = useState<PriceChange[]>([])
	const { hosts } = props
	const network = (window.location.pathname.toLowerCase().indexOf('zen') >= 0 ? 'zen' : 'mainnet')
	const [loadingHost, setLoadingHost] = useState(false)
    const [loadingPriceChanges, setLoadingPriceChanges] = useState(false)
    const [node, setNode] = useState('global')
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
                    </div>
                    <div className="host-details-subcontainer">
                        <HostPrices
                            darkMode={props.darkMode}
                            data={priceChanges}
                        />
                        <HostResults
                            darkMode={props.darkMode}
                            host={host}
                            node={node}
                        />
                    </div>
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
