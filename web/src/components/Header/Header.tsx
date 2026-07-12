import './Header.css'
import { Logo, ModeSelector, NetworkSelector } from '../'
import { useContext } from 'react'
import { NetworkContext } from '../../contexts'

type HeaderProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

export const Header = (props: HeaderProps) => {
	const { network, switchNetwork } = useContext(NetworkContext)
	return (
		<header className="header-container">
			<div className="wrap header-inner">
				<Logo/>
				<div className="header-right">
					<NetworkSelector
						darkMode={props.darkMode}
						network={network}
						switchNetwork={switchNetwork}
					/>
					<ModeSelector
						darkMode={props.darkMode}
						toggleDarkMode={props.toggleDarkMode}
					/>
				</div>
			</div>
		</header>
	)
}
