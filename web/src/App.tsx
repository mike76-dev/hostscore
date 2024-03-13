import { useState, useEffect } from 'react'
import {
	createBrowserRouter,
	RouterProvider,
	Outlet
} from 'react-router-dom'
import Header from './components/Header'
import Footer from './components/Footer'
import Content from './components/Content'
import About from './components/About'
import Hosts from './components/Hosts'
import { NetworkContext } from './contexts'

const App = () => {
	let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : false
	const [darkMode, toggleDarkMode] = useState(mode)
	const [network, switchNetwork] = useState('')
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
								<Hosts network="mainnet" darkMode={darkMode}/>
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
								<Hosts network="zen" darkMode={darkMode}/>
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
			<RouterProvider router={router}/>
		</NetworkContext.Provider>
	);
}

export default App;
