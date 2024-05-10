import './ModeSelector.css'
import { useState, useEffect } from 'react'
import darkMode from '../../assets/dark_mode.png'
import lightMode from '../../assets/light_mode.png'

type ModeSelectorProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

export const ModeSelector = (props: ModeSelectorProps) => {
	const [mode, toggleDarkMode] = useState(props.darkMode)
	useEffect(() => {
		toggleDarkMode(props.darkMode)
	}, [props.darkMode])
	return (
		// eslint-disable-next-line
		<a
			className="mode-selector-switch"
			tabIndex={1}
			onClick={() => {
				props.toggleDarkMode(!mode)
			}}
			onKeyUp={(event: React.KeyboardEvent<HTMLAnchorElement>) => {
				if (event.key === 'Enter' || event.key === ' ') {
					props.toggleDarkMode(!mode)
				}
			}}
		>
			<div className="mode-selector-container">
				<img
					className="mode-selector-icon"
					src={mode ? lightMode : darkMode}
					alt={mode ? 'Light Mode' : 'Dark Mode'}
				/>
			</div>
		</a>
	)
}