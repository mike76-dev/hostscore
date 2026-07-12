import { useState, useEffect } from 'react'
import {
	Content,
	Footer,
	Header
} from './components'

const App = () => {
	let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : window.matchMedia('(prefers-color-scheme: dark)').matches
	const [darkMode, toggleDarkMode] = useState(mode)
	useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
		document.documentElement.dataset.theme = darkMode ? 'dark' : 'light'
	}, [darkMode])
	return (
		<>
			<Header
				darkMode={darkMode}
				toggleDarkMode={toggleDarkMode}
			/>
			<Content darkMode={darkMode}/>
			<Footer darkMode={darkMode}/>
		</>
	)
}

export default App
