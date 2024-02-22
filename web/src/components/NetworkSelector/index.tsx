import './index.css'
import React, { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

type NetworkSelectorProps = {
    network: string,
    switchNetwork: (network: string) => any,
}

const NetworkSelector = (props: NetworkSelectorProps) => {
    const navigate = useNavigate()
    const [network, switchNetwork] = useState(props.network)
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
            >
                <option value="mainnet">Mainnet</option>
                <option value="zen">Zen</option>
            </select>
        </div>
    )
}

export default NetworkSelector
