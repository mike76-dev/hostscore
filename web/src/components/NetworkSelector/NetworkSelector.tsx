import './NetworkSelector.css'
import React, { useState, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useExcludedPaths, getNetworkHosts } from '../../api'

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
    const [onlineHosts, setOnlineHosts] = useState(0)
    const [time, setTime] = useState(new Date())
	useEffect((): any => {
		const interval = setInterval(() => {
			setTime(new Date())
		}, 600000)
		return () => clearInterval(interval)
	}, [])
	useEffect(() => {
		if (excludedPaths.includes(location.pathname)) return
		if (location.pathname.indexOf('/zen') === 0) {
			switchNetwork('zen')
		} else switchNetwork('mainnet')
	}, [location, excludedPaths])
	useEffect(() => {
		switchNetwork(props.network)
	}, [props.network])
    useEffect(() => {
        if (network === '') return
        getNetworkHosts(network)
        .then(data => {
            if (data) setOnlineHosts(data.hosts.online)
        })
    }, [network, time])
	return (
		<div className={'network-selector-container' + (props.darkMode ? ' network-selector-dark' : '')}>
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
            <div className="network-selector-text">
                {network !== '' && 'Online hosts: ' + onlineHosts}
            </div>
		</div>
	)
}
