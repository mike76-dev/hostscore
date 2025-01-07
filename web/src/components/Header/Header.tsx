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
		<div className={'header-container' + (props.darkMode ? ' header-dark-mode' : '')}>
			<Logo/>
			<ModeSelector
				darkMode={props.darkMode}
				toggleDarkMode={props.toggleDarkMode}
			/>
			<NetworkSelector
				darkMode={props.darkMode}
				network={network}
				switchNetwork={switchNetwork}
			/>
		</div>
	)
}
