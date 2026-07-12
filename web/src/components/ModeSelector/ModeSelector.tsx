import './ModeSelector.css'

type ModeSelectorProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

export const ModeSelector = (props: ModeSelectorProps) => {
	return (
		<button
			className="icon-btn"
			tabIndex={1}
			aria-label={props.darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
			onClick={() => props.toggleDarkMode(!props.darkMode)}
		>
			{props.darkMode ?
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
					<circle cx="12" cy="12" r="4"/>
					<path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4m11.4-11.4 1.4-1.4"/>
				</svg>
			:
				<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
					<path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/>
				</svg>
			}
		</button>
	)
}
