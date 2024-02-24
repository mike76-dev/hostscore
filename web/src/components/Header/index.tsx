import './index.css'
import Logo from '../Logo'
import ModeSelector from '../ModeSelector'
import NetworkSelector from '../NetworkSelector'
import { useContext } from 'react'
import { NetworkContext } from '../../contexts'

type HeaderProps = {
	darkMode: boolean,
	toggleDarkMode: (mode: boolean) => any,
}

const Header = (props: HeaderProps) => {
	const { network, switchNetwork } = useContext(NetworkContext)
	return (
		<div className={'header-container' + (props.darkMode ? ' header-dark-mode' : '')}>
			<Logo/>
			<ModeSelector
				darkMode={props.darkMode}
				toggleDarkMode={props.toggleDarkMode}
			/>
			<NetworkSelector
				network={network}
				switchNetwork={switchNetwork}
			/>
		</div>
	)
}

export default Header