import React, { useState, useEffect } from 'react';
import {
	createBrowserRouter,
	RouterProvider,
} from 'react-router-dom'
import Header from './components/Header'
import Footer from './components/Footer'
import Content from './components/Content'
import About from './components/About';

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
					<Content darkMode={darkMode}>
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
						network={network}
						switchNetwork={switchNetwork}
						darkMode={darkMode}
						toggleDarkMode={toggleDarkMode}
					/>
					<Content darkMode={darkMode}/>
					<Footer darkMode={darkMode}/>
				</>
			),
			
		},
		{
			path: 'about',
			element: (
				<>
					<Header
						network={network}
						switchNetwork={switchNetwork}
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
	])

	return (
		<RouterProvider router={router}/>
	);
}

export default App;
