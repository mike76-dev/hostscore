import './HostDetails.css'
import { useState, useEffect } from 'react'
import { useParams } from 'react-router'
import { useNavigate } from 'react-router-dom'
import {
    Host,
    HostScan,
    HostBenchmark,
    getHost,
    getScans,
    getBenchmarks,
    stripePrefix,
    useLocations
} from '../../api'
import {
    Button,
    HostInfo,
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
    const [scans, setScans] = useState<HostScan[]>([])
    const [benchmarks, setBenchmarks] = useState<HostBenchmark[]>([])
	const { hosts } = props
	const network = (window.location.pathname.toLowerCase().indexOf('zen') >= 0 ? 'zen' : 'mainnet')
	const [loadingHost, setLoadingHost] = useState(false)
    const [loadingScans, setLoadingScans] = useState(false)
    const [loadingBenchmarks, setLoadingBenchmarks] = useState(false)
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
        setLoadingScans(true)
        getScans(network, publicKey || '', undefined, undefined, 48 * locations.length, true)
        .then(data => {
            if (data && data.status === 'ok' && data.scans) {
                setScans(data.scans)
            }
            setLoadingScans(false)
        })
        setLoadingBenchmarks(true)
        getBenchmarks(network, publicKey || '', undefined, undefined, 12 * locations.length, false)
        .then(data => {
            if (data && data.status === 'ok' && data.benchmarks) {
                setBenchmarks(data.benchmarks)
            }
            setLoadingBenchmarks(false)
        })
    }, [network, publicKey, locations.length])
	return (
		<div className={'host-details-container' + (props.darkMode ? ' host-details-dark' : '')}>
			{loadingHost || loadingScans || loadingBenchmarks ?
				<Loader darkMode={props.darkMode}/>
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
                    </div>
                    <HostResults
                        darkMode={props.darkMode}
                        scans={scans}
                        benchmarks={benchmarks}
                        node={node}
                    />
                </>
			:
				<div className="host-not-found">Host Not Found</div>
			)}
            <Button
				icon={Back}
				caption="back"
				darkMode={props.darkMode}
				onClick={() => {navigate(-1)}}
			/>
		</div>
	)
}
