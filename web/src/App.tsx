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
    Hosts,
    HostDetails
} from './components'
import { Host } from './api'
import { NetworkContext, HostContext } from './contexts'

const App = () => {
	let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : false
	const [darkMode, toggleDarkMode] = useState(mode)
	const [network, switchNetwork] = useState('')
	const [hosts, setHosts] = useState<Host[]>([])
	useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
	}, [darkMode])
	useEffect(() => {
		if (window.location.pathname === '/about') return
		if (window.location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else {
			switchNetwork('mainnet')
		}
	}, [])
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
			],
		},
	])

	return (
		<NetworkContext.Provider value={{ network, switchNetwork }}>
			<HostContext.Provider value={{ hosts, setHosts }}>
				<RouterProvider router={router}/>
			</HostContext.Provider>
		</NetworkContext.Provider>
	);
}

export default App;
