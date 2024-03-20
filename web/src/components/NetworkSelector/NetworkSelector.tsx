import './NetworkSelector.css'
import React, { useState, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'

type NetworkSelectorProps = {
	network: string,
	switchNetwork: (network: string) => any,
}

export const NetworkSelector = (props: NetworkSelectorProps) => {
	const location = useLocation()
	const navigate = useNavigate()
	const [network, switchNetwork] = useState(props.network)
	useEffect(() => {
		if (location.pathname === '/about') return
		if (location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else switchNetwork('mainnet')
	}, [location])
	useEffect(() => {
		switchNetwork(props.network)
	}, [props.network])
	return (
		<div className="network-selector-container">
			<select
				className="network-selector-select"
				onChange={(event: React.ChangeEvent<HTMLSelectElement>) => {
					props.switchNetwork(event.target.value)
					navigate(event.target.value === 'mainnet' ? '/' : '/zen')
				}}
				value={network}
				tabIndex={1}
			>
				<option value="mainnet">Mainnet</option>
				<option value="zen">Zen</option>
			</select>
		</div>
	)
}
