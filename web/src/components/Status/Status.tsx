import './Status.css'
import { useState, useEffect, useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Loader } from '../'
import Back from '../../assets/back.png'
import { NodeStatus, getStatus, useLocations } from '../../api'
import { NetworkContext } from '../../contexts'

type StatusProps = { darkMode: boolean }

export const Status = (props: StatusProps) => {
	const navigate = useNavigate()
    const locations = useLocations()
    const { network } = useContext(NetworkContext)
	const [version, setVersion] = useState('')
	const [nodes, setNodes] = useState<{ [node: string]: NodeStatus }>()
	const [time, setTime] = useState(new Date())
    const [loading, setLoading] = useState(false)
	useEffect((): any => {
		const interval = setInterval(() => {
			setTime(new Date())
		}, 600000)
		return () => clearInterval(interval)
	}, [])
	useEffect(() => {
        setLoading(true)
		getStatus()
		.then(data => {
			if (data && data.status === 'ok') {
				setVersion(data.version)
				setNodes(data.nodes)
			} else {
				setVersion('0.0.0')
			}
            setLoading(false)
		})
	}, [time])
	const getStyle = (balance: string) => {
		switch (balance) {
			case 'ok': return 'good'
			case 'low': return 'medium'
			default: return 'bad'
		}
	}
	return (
		<div className={'status-container' + (props.darkMode ? ' status-container-dark' : '')}>
			<h1>Service Status</h1>
            {loading ?
                <Loader
                    darkMode={props.darkMode}
                    className="status-loader"
                />
            :
			    (version === '0.0.0' ?
    				<div className="status-unavailable">Temporarily unavailable.</div>
	    		:
		    		<table>
			    		<tbody>
				    		<tr>
					    		<th colSpan={2}>Node:</th>
						    	{nodes && locations.map(location => (
							    	<th key={'header-' + location.short}>{location.long}</th>
    							))}
	    					</tr>
		    				<tr>
			    				<th colSpan={2}>Online:</th>
				    			{nodes && locations.map(location => (
					    			<td key={'online-' + location.short}>
						    			<div className={'status' + (nodes[location.short].online === true ? ' status-good' : ' status-bad')}></div>
							    	</td>
    							))}
	    					</tr>
		    				<tr>
			    				<th colSpan={2}>Version:</th>
				    			{nodes && locations.map(location => (
                                    <td key={'version-' + location.short}>
                                        {nodes[location.short].online === true ? nodes[location.short].version : ''}
                                    </td>
						    	))}
    						</tr>
	    					<tr>
		    					<th rowSpan={2}>Mainnet:</th>
			    				<td>Height</td>
				    			{nodes && locations.map(location => (
					    			<td key={'height-mainnet-' + location.short}>
                                        {nodes[location.short].online === true ? nodes[location.short].networks['mainnet'].height : ''}
                                    </td>
						    	))}
    						</tr>
	    					<tr>
		    					<td>Balance</td>
			    				{nodes && locations.map(location => (
    				    			<td key={'balance-mainnet-' + location.short}>
                                        {nodes[location.short].online === true &&
	    				    				<div className={'status status-' + getStyle(nodes[location.short].networks['mainnet'].balance)}></div>
                                        }
		    				    	</td>
							    ))}
    						</tr>
	    					<tr>
		    					<th rowSpan={2}>Zen:</th>
			    				<td>Height</td>
				    			{nodes && locations.map(location => (
					    			<td key={'height-zen-' + location.short}>
                                        {nodes[location.short].online === true ? nodes[location.short].networks['zen'].height : ''}
                                    </td>
						    	))}
    						</tr>
	    					<tr>
		    					<td>Balance</td>
			    				{nodes && locations.map(location => (
    				    			<td key={'balance-zen-' + location.short}>
                                        {nodes[location.short].online === true &&
	    				    				<div className={'status status-' + getStyle(nodes[location.short].networks['zen'].balance)}></div>
                                        }
		    				    	</td>
							    ))}
    						</tr>
	    					<tr><td colSpan={locations.length + 2}></td></tr>
		    				<tr>
			    				<th colSpan={2}>Portal Version:</th>
				    			<td colSpan={locations.length}>{version}</td>
					    	</tr>
    					</tbody>
	    			</table>
                )
			}
			<Button
				icon={Back}
				caption="home"
				darkMode={props.darkMode}
				onClick={() => {navigate(network === 'zen' ? '/zen' : '/')}}
			/>
		</div>
	)
}