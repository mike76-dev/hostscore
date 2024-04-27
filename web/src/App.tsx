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
    const [offset, changeOffset] = useState(0)
    const [limit, changeLimit] = useState(10)
    const [onlineOnly, setOnlineOnly] = useState(true)
    const [query, setQuery] = useState('')
    const [sorting, changeSorting] = useState<HostSortType>({ sortBy: 'rank', order: 'asc' })
    const [countries, setCountries] = useState<string[]>([])
    const [country, setCountry] = useState('')
    const excludedPaths = useExcludedPaths()
	useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
	}, [darkMode])
	useEffect(() => {
		if (excludedPaths.includes(window.location.pathname)) return
		if (window.location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else {
			switchNetwork('mainnet')
		}
	}, [excludedPaths])
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
