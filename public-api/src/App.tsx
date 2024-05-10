import { useState, useEffect } from 'react'
import {
    Content,
    Footer,
    Header
} from './components'

const App = () => {
    let data = window.localStorage.getItem('darkMode')
	let mode = data ? JSON.parse(data) : false
	const [darkMode, toggleDarkMode] = useState(mode)
    useEffect(() => {
		window.localStorage.setItem('darkMode', JSON.stringify(darkMode))
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
