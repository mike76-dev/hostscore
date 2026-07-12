import './Hosts.css'
import { useRef, useState, useEffect, useContext } from 'react'
import axios from 'axios'
import {
	Averages,
	CountrySelector,
	HostSelector,
	HostSearch,
	HostNavigation,
	HostsTable,
	HostMap,
	Loader
} from '../'
import {
	Host,
	HostCount,
	getHosts,
	getAverages,
	getCountries,
	getNetworkHosts,
	toSia
} from '../../api'
import { HostContext, NetworkContext } from '../../contexts'

type HostsProps = {
	network: string,
	darkMode: boolean,
	setHosts: (hosts: Host[]) => any,
}

const formatCount = (count: number) => count.toLocaleString('en-US')

const formatPB = (bytes: number) => (bytes / 1e15).toFixed(2)

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
		changeSorting,
		countries,
		setCountries,
		country,
		setCountry
	} = useContext(HostContext)
	const switchHosts = (value: string) => {setOnlineOnly(value === 'online')}
	const [hosts, setHostsLocal] = useState<Host[]>([])
	const [mapHosts, setMapHosts] = useState<Host[]>([])
	const [total, setTotal] = useState(0)
	const [hostCount, setHostCount] = useState<HostCount>()
	const [loading, setLoading] = useState(false)
	const [loadingAverages, setLoadingAverages] = useState(false)
	const { network, setHosts } = props
	const prevOnlineOnly = useRef(onlineOnly)
	const prevQuery = useRef(query)
	const prevCountry = useRef(country)
	const prevSorting = useRef(sorting)
	const [time, setTime] = useState(new Date())
	const { averages, setAverages } = useContext(NetworkContext)
	const prevNetwork = useRef(network)
	useEffect((): any => {
		const interval = setInterval(() => {
			setTime(new Date())
		}, 600000)
		return () => clearInterval(interval)
	}, [])
	useEffect(() => {
		if (
			prevOnlineOnly.current !== onlineOnly ||
			prevQuery.current !== query ||
			prevCountry.current !== country ||
			prevSorting.current !== sorting
		) {
			changeOffset(0)
			prevOnlineOnly.current = onlineOnly
			prevQuery.current = query
			prevCountry.current = country
			prevSorting.current = sorting
		}
	}, [onlineOnly, query, country, sorting, changeOffset])
	useEffect(() => {
		const cancelTokenSource = axios.CancelToken.source()
		getHosts(network, false, 0, -1, query, country, { sortBy: 'rank', order: 'asc' }, cancelTokenSource.token)
		.then(data => {
			if (data && data.hosts) {
				setMapHosts(data.hosts)
			}
		})
		return () => {
			cancelTokenSource.cancel('request canceled')
		}
	}, [network, query, country, time])
	useEffect(() => {
		setLoading(true)
		const cancelTokenSource = axios.CancelToken.source()
		getHosts(network, !onlineOnly, offset, limit, query, country, sorting, cancelTokenSource.token)
		.then(data => {
			if (data && data.hosts) {
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
		return () => {
			cancelTokenSource.cancel('request canceled')
		}
	}, [network, onlineOnly, offset, limit, query, country, sorting, setHosts])
	useEffect(() => {
		setLoadingAverages(true)
		getAverages(network)
		.then(data => {
			if (data && data.averages) {
				setAverages(data.averages)
			}
			setLoadingAverages(false)
		})
	}, [network, time, setAverages, setLoadingAverages])
	useEffect(() => {
		getNetworkHosts(network)
		.then(data => {
			if (data) setHostCount(data.hosts)
		})
	}, [network, time])
	useEffect(() => {
		getCountries(network, !onlineOnly)
		.then(data => {
			if (data && data.countries) {
				setCountries(data.countries)
			}
		})
	}, [network, time, onlineOnly, setCountries])
	useEffect(() => {
		if (prevNetwork.current !== network) {
			changeOffset(0)
			setOnlineOnly(true)
			setQuery('')
			prevNetwork.current = network
		}
	// eslint-disable-next-line
	}, [network, changeOffset])

	// Network-wide statistics derived from the full list of online hosts.
	const accepting = mapHosts.filter(host =>
		host.v2 === true && host.v2Settings.acceptingContracts).length
	const totalStorage = mapHosts.reduce((sum, host) =>
		sum + (host.v2 === true ? host.v2Settings.totalStorage * 4 * 1024 * 1024 : 0), 0)
	const usedStorage = mapHosts.reduce((sum, host) =>
		sum + (host.v2 === true ?
			(host.v2Settings.totalStorage - host.v2Settings.remainingStorage) * 4 * 1024 * 1024 : 0), 0)
	const tier1 = averages['tier1']

	return (
		<div className="hosts-container">
			<section className="hosts-hero">
				<div className="eyebrow">
					{'Sia storage network · ' + (network === 'zen' ? 'Zen testnet' : 'Mainnet')}
				</div>
				<h1 className="hosts-title"><em>Every host</em> under continuous observation</h1>
				<p className="hosts-lead">
					HostScore watches every host on the Sia network, scanning them
					around the clock from three vantage points — Europe, East&nbsp;USA
					and Asia. Hosts are benchmarked and scored on pricing, performance
					and reliability.
				</p>
				<div className="hosts-stats">
					<div className="hosts-stat">
						<div className="hosts-stat-key">Known V2 hosts</div>
						<div className="hosts-stat-value">
							{hostCount && hostCount.totalV2 !== undefined ? formatCount(hostCount.totalV2) : '—'}
						</div>
						<div className="hosts-stat-sub">
							{hostCount ? 'of ' + formatCount(hostCount.total) + ' hosts ever seen' : ' '}
						</div>
					</div>
					<div className="hosts-stat">
						<div className="hosts-stat-key">Online hosts</div>
						<div className="hosts-stat-value">
							{hostCount ? formatCount(hostCount.online) : '—'}
						</div>
						<div className="hosts-stat-sub">
							{mapHosts.length > 0 ?
								formatCount(accepting) + ' accepting contracts · ' +
								(accepting * 100 / mapHosts.length).toFixed(1) + '%' : ' '}
						</div>
					</div>
					<div className="hosts-stat">
						<div className="hosts-stat-key">Advertised capacity</div>
						<div className="hosts-stat-value">
							{mapHosts.length > 0 ? <>{formatPB(totalStorage)} <small>PB</small></> : '—'}
						</div>
						<div className="hosts-stat-sub">
							{mapHosts.length > 0 ?
								formatPB(usedStorage) + ' PB in use · ' +
								(totalStorage > 0 ? (usedStorage * 100 / totalStorage).toFixed(0) : '0') + '%' : ' '}
						</div>
					</div>
					<div className="hosts-stat">
						<div className="hosts-stat-key">Tier-1 storage price</div>
						<div className="hosts-stat-value">
							{tier1 && tier1.available ? toSia(tier1.storagePrice, true) : '—'}
						</div>
						<div className="hosts-stat-sub">avg of top 10 hosts, /TB·month</div>
					</div>
				</div>
			</section>
			<div className="hosts-cols">
				<div className="panel hosts-map-panel">
					<div className="panel-h">
						<h2>Host map</h2>
					</div>
					<div className="hosts-map-body">
						<HostMap
							darkMode={props.darkMode}
							network={network}
							hosts={mapHosts}
						/>
					</div>
				</div>
				<div className="panel hosts-averages-panel">
					{loadingAverages ?
						<Loader
							darkMode={props.darkMode}
							className="hosts-averages-loader"
						/>
					:
						<Averages
							darkMode={props.darkMode}
							averages={averages}
						/>
					}
				</div>
			</div>
			<div className="hosts-section-h">
				<h2>Host explorer</h2>
				<span className="hosts-section-sub">
					{hostCount ? formatCount(hostCount.online) + ' online · ranked by total score' : ''}
				</span>
			</div>
			<div className="panel hosts-explorer">
				<div className="hosts-filters">
					<HostSearch
						darkMode={props.darkMode}
						value={query}
						onChange={setQuery}
					/>
					<CountrySelector
						darkMode={props.darkMode}
						options={countries}
						value={country}
						onChange={setCountry}
					/>
					<HostSelector
						darkMode={props.darkMode}
						value={onlineOnly ? 'online' : 'all'}
						onChange={switchHosts}
					/>
				</div>
				<div className="hosts-table-div">
					{loading ?
						<Loader
							darkMode={props.darkMode}
							className="hosts-table-loader"
						/>
					:
					(hosts.length > 0 ?
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
					:
						<div className="hosts-not-found">No hosts found</div>
					)}
				</div>
			</div>
		</div>
	)
}
