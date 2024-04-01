import './Hosts.css'
import { useRef, useState, useEffect, useContext } from 'react'
import {
    HostSelector,
    HostSearch,
    HostNavigation,
    HostsTable,
    Loader
} from '../'
import { Host, getHosts } from '../../api'
import { HostContext } from '../../contexts'

type HostsProps = {
	network: string,
	darkMode: boolean,
	setHosts: (hosts: Host[]) => any,
}

export const Hosts = (props: HostsProps) => {
    const {
        offset,
        changeOffset,
        limit,
        changeLimit,
        onlineOnly,
        setOnlineOnly,
        query,
        setQuery,
        sorting,
        changeSorting
    } = useContext(HostContext)
    const prevOnlineOnly = useRef(onlineOnly)
	const switchHosts = (value: string) => {setOnlineOnly(value === 'online')}
	const [hosts, setHostsLocal] = useState<Host[]>([])
	const [total, setTotal] = useState(0)
	const [loading, setLoading] = useState(false)
	const { network, setHosts } = props
    const prevQuery = useRef(query)
    useEffect(() => {
        if (prevOnlineOnly.current !== onlineOnly || prevQuery.current !== query) {
            changeOffset(0)
            prevOnlineOnly.current = onlineOnly
            prevQuery.current = query
        }
    }, [onlineOnly, query, changeOffset])
	useEffect(() => {
		setLoading(true)
		getHosts(network, !onlineOnly, offset, limit, query, sorting)
		.then(data => {
			if (data && data.status === 'ok' && data.hosts) {
				setHostsLocal(data.hosts)
				setTotal(data.total)
				setHosts(data.hosts)
			} else {
				setHostsLocal([])
				setTotal(0)
				setHosts([])
			}
			setLoading(false)
		})
	}, [network, onlineOnly, offset, limit, query, sorting, setHosts])
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
			{hosts.length > 0 ?
				<>
					<HostsTable
						darkMode={props.darkMode}
						hosts={hosts}
                        sorting={sorting}
                        changeSorting={changeSorting}
					/>
					<HostNavigation
						darkMode={props.darkMode}
						offset={offset}
						limit={limit}
						total={total}
						changeOffset={changeOffset}
						changeLimit={changeLimit}
					/>
				</>
            : <div className={'hosts-not-found' + (props.darkMode ? ' hosts-not-found-dark' : '')}>No hosts found</div>
			}
		</div>
	)
}
