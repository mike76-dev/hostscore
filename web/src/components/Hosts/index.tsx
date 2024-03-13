import './index.css'
import { useState, useEffect } from 'react'
import HostSelector from '../HostSelector'
import HostSearch from '../HostSearch'
import HostNavigation from '../HostNavigation'
import HostsTable from '../HostsTable'
import Loader from '../Loader'
import { Host, getHosts } from '../../api'

type HostsProps = {
	network: string,
	darkMode: boolean
}

const Hosts = (props: HostsProps) => {
	const [onlineOnly, setOnlineOnly] = useState(true)
	const switchHosts = (value: string) => {setOnlineOnly(value === 'online')}
	const [query, setQuery] = useState('')
	const [offset, changeOfset] = useState(0)
	const [limit, changeLimit] = useState(10)
	const [hosts, setHosts] = useState<Host[]>([])
	const [total, setTotal] = useState(0)
	const [loading, setLoading] = useState(false)
	useEffect(() => {
		setLoading(true)
		getHosts(props.network, !onlineOnly, offset, limit, query)
		.then(data => {
			if (data && data.status === 'ok' && data.hosts) {
				setHosts(data.hosts)
				setTotal(data.total)
			} else {
				setHosts([])
				setTotal(0)
			}
			setLoading(false)
		})
	}, [props.network, onlineOnly, offset, limit, query])
	return (
		<div className="hosts-container">
			{loading &&
				<Loader darkMode={props.darkMode}/>
			}
			<HostSelector
				darkMode={props.darkMode}
				value={onlineOnly ? 'online' : 'all'}
				onChange={switchHosts}
			/>
			<HostSearch
				darkMode={props.darkMode}
				value={query}
				onChange={setQuery}
			/>
			{hosts.length > 0 &&
				<>
					<HostsTable
						darkMode={props.darkMode}
						hosts={hosts}
					/>
					<HostNavigation
						darkMode={props.darkMode}
						offset={offset}
						limit={limit}
						total={total}
						changeOffset={changeOfset}
						changeLimit={changeLimit}
					/>
				</>
			}
		</div>
	)
}

export default Hosts