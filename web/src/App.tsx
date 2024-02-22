import React, { useState, useEffect } from 'react';
import {
	createBrowserRouter,
	RouterProvider,
} from 'react-router-dom'
import Header from './components/Header'

function App() {
	let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : false
	const [darkMode, toggleDarkMode] = useState(mode)
	const [network, switchNetwork] = useState(window.location.pathname === '/' ? 'mainnet' : 'zen')
	useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
	}, [darkMode])
	const router = createBrowserRouter([
		{
			path: '/',
			element: (
				<>
					<Header
						network={network}
						switchNetwork={switchNetwork}
						darkMode={darkMode}
						toggleDarkMode={toggleDarkMode}
					/>
				</>
			),
		},
		{
			path: 'zen',
			element: (
				<>
					<Header
						network={network}
						switchNetwork={switchNetwork}
						darkMode={darkMode}
						toggleDarkMode={toggleDarkMode}
					/>
				</>
			),
		},
	])

	return (
		<RouterProvider router={router}/>
	);
}

export default App;
