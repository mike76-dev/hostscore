import './Hosts.css'
import { useRef, useState, useEffect, useContext } from 'react'
import {
    Averages,
    HostSelector,
    HostSearch,
    HostNavigation,
    HostsTable,
    HostMap,
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
    const [loadingAverages, setLoadingAverages] = useState(false)
	const { network, setHosts } = props
    const prevQuery = useRef(query)
    const [totalHosts, setTotalHosts] = useState<Host[]>([])
    const [time, setTime] = useState(new Date())
	useEffect((): any => {
		const interval = setInterval(() => {
			setTime(new Date())
		}, 600000)
		return () => clearInterval(interval)
	}, [])
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
    useEffect(() => {
        setLoadingAverages(true)
        getHosts(network, false, 0, -1, '', { sortBy: 'rank', order: 'asc' })
        .then(data => {
            if (data && data.status === 'ok' && data.hosts) {
                setTotalHosts(data.hosts)
            } else {
                setTotalHosts([])
            }
            setLoadingAverages(false)
        })
    }, [network, time, setTotalHosts])
	return (
		<div className="hosts-container">
            <div className="hosts-subcontainer">
                <HostMap
                    darkMode={props.darkMode}
                    network={network}
                    query={query}
                />
                <div className="hosts-table-div">
                    {loading &&
	    			    <Loader
                            darkMode={props.darkMode}
                            className="hosts-table-loader"
                        />
        			}
                    <div className="hosts-row">
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
                    </div>
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
            </div>
            <div className="hosts-averages-div">
                {loadingAverages &&
                    <Loader
                        darkMode={props.darkMode}
                        className="hosts-averages-loader"
                    />
                }
                {totalHosts.length > 0 &&
                    <Averages
                        darkMode={props.darkMode}
                        hosts={totalHosts}
                    />
                }
            </div>
		</div>
	)
}
