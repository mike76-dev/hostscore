import './NetworkSelector.css'
import { useState, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useExcludedPaths } from '../../api'

type NetworkSelectorProps = {
	darkMode: boolean,
	network: string,
	switchNetwork: (network: string) => any,
}

export const NetworkSelector = (props: NetworkSelectorProps) => {
	const location = useLocation()
	const navigate = useNavigate()
	const [network, switchNetwork] = useState(props.network)
	const excludedPaths = useExcludedPaths()
	useEffect(() => {
		if (excludedPaths.includes(location.pathname)) return
		if (location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else switchNetwork('mainnet')
	}, [location, excludedPaths])
	useEffect(() => {
		switchNetwork(props.network)
	}, [props.network])
	const select = (value: string) => {
		if (value === network) return
		props.switchNetwork(value)
		navigate(value === 'zen' ? '/zen' : '/')
	}
	return (
		<div className="seg" role="group" aria-label="Network">
			<button
				tabIndex={1}
				aria-pressed={network !== 'zen'}
				onClick={() => select('mainnet')}
			>Mainnet</button>
			<button
				tabIndex={1}
				aria-pressed={network === 'zen'}
				onClick={() => select('zen')}
			>Zen</button>
		</div>
	)
}
