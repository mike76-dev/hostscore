import './index.css'
import Logo from '../Logo'
import ModeSelector from '../ModeSelector'
import NetworkSelector from '../NetworkSelector'

type HeaderProps = {
	network: string,
	switchNetwork: (network: string) => any,
	darkMode: boolean
	toggleDarkMode: (mode: boolean) => any,
}

const Header = (props: HeaderProps) => {
	return (
		<div className={'header-container' + (props.darkMode ? ' header-dark-mode' : '')}>
			<Logo/>
			<ModeSelector
				darkMode={props.darkMode}
				toggleDarkMode={props.toggleDarkMode}
			/>
			<NetworkSelector
				network={props.network}
				switchNetwork={props.switchNetwork}
			/>
		</div>
	)
}

export default Header