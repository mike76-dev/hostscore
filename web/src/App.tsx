import { useState, useEffect } from 'react'
import {
	createBrowserRouter,
	RouterProvider,
	Outlet
} from 'react-router-dom'
import {
	Header,
	Footer,
	Content,
	About,
	FAQ,
	Hosts,
	HostDetails,
	Status
} from './components'
import {
	Host,
	useExcludedPaths,
	HostSortType,
	NetworkAverages
} from './api'
import { NetworkContext, HostContext } from './contexts'

const App = () => {
	let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : false
	const [darkMode, toggleDarkMode] = useState(mode)
	const [network, switchNetwork] = useState('')
	const [averages, setAverages] = useState<{ [tier: string]: NetworkAverages }>({})
	const [hosts, setHosts] = useState<Host[]>([])
	const [offset, changeOffset] = useState(() => {
		const savedOffset = window.localStorage.getItem('offset')
		return savedOffset ? parseInt(savedOffset, 10) : 0
	})
	const [limit, changeLimit] = useState(() => {
		const savedLimit = window.localStorage.getItem('limit')
		return savedLimit ? parseInt(savedLimit, 10) : 10
	})
	const [onlineOnly, setOnlineOnly] = useState(() => {
		const savedOnlineOnly = window.localStorage.getItem('online')
		return savedOnlineOnly && savedOnlineOnly === 'false' ? false : true
	})
	const [query, setQuery] = useState(() => {
		return window.localStorage.getItem('query') || ''
	})
	const [sorting, changeSorting] = useState<HostSortType>(() => {
		const savedSorting = window.localStorage.getItem('sorting')
		return savedSorting ? JSON.parse(savedSorting) : { sortBy: 'rank', order: 'asc' }
	})
	const [countries, setCountries] = useState<string[]>([])
	const [country, setCountry] = useState(() => {
		return window.localStorage.getItem('country') || ''
	})
	const excludedPaths = useExcludedPaths()
	useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
	}, [darkMode])
	useEffect(() => {
		if (excludedPaths.includes(window.location.pathname)) return
		if (window.location.pathname.indexOf('/anagami') === 0) {
			switchNetwork('anagami')
		} else if (window.location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else {
			switchNetwork('mainnet')
		}
	}, [excludedPaths])
	useEffect(() => {
		window.localStorage.setItem('offset', offset.toString())
	}, [offset])
	useEffect(() => {
		window.localStorage.setItem('limit', limit.toString())
	}, [limit])
	useEffect(() => {
		window.localStorage.setItem('online', onlineOnly ? 'true' : 'false')
	}, [onlineOnly])
	useEffect(() => {
		window.localStorage.setItem('query', query)
	}, [query])
	useEffect(() => {
		window.localStorage.setItem('sorting', JSON.stringify(sorting))
	}, [sorting])
	useEffect(() => {
		window.localStorage.setItem('country', country)
	}, [country])
	const router = createBrowserRouter([
		{
			element: <Outlet/>,
			children: [
				{
					path: '/',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<Hosts
									network="mainnet"
									darkMode={darkMode}
									setHosts={setHosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'zen',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<Hosts
									network="zen"
									darkMode={darkMode}
									setHosts={setHosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'anagami',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<Hosts
									network="anagami"
									darkMode={darkMode}
									setHosts={setHosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'host/:publicKey',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<HostDetails
									darkMode={darkMode}
									hosts={hosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'zen/host/:publicKey',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<HostDetails
									darkMode={darkMode}
									hosts={hosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'anagami/host/:publicKey',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<HostDetails
									darkMode={darkMode}
									hosts={hosts}
								/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'about',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<About darkMode={darkMode}/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'faq',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<FAQ darkMode={darkMode}/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'faq/:link',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<FAQ darkMode={darkMode}/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
				{
					path: 'status',
					element: (
						<>
							<Header
								darkMode={darkMode}
								toggleDarkMode={toggleDarkMode}
							/>
							<Content darkMode={darkMode}>
								<Status darkMode={darkMode}/>
							</Content>
							<Footer darkMode={darkMode}/>
						</>
					),
				},
			],
		},
	])

	return (
		<NetworkContext.Provider value={{
			network,
			switchNetwork,
			averages,
			setAverages
		}}>
			<HostContext.Provider value={{
				hosts,
				setHosts,
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
			}}>
				<RouterProvider router={router}/>
			</HostContext.Provider>
		</NetworkContext.Provider>
	);
}

export default App;
