import './Header.css'
import { Logo, ModeSelector } from '../'

type HeaderProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

export const Header = (props: HeaderProps) => {
	return (
		<header className="header-container">
			<div className="wrap header-inner">
				<Logo/>
				<div className="header-right">
					<ModeSelector
						darkMode={props.darkMode}
						toggleDarkMode={props.toggleDarkMode}
					/>
				</div>
			</div>
		</header>
	)
}
