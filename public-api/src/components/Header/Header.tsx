import './Header.css'
import { Logo, ModeSelector } from '../'

type HeaderProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

export const Header = (props: HeaderProps) => {
	return (
		<div className={'header-container' + (props.darkMode ? ' header-dark-mode' : '')}>
			<Logo/>
			<ModeSelector
				darkMode={props.darkMode}
				toggleDarkMode={props.toggleDarkMode}
			/>
		</div>
	)
}