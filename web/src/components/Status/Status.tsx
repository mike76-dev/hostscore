import './Status.css'
import { useState, useEffect, useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Loader } from '../'
import Back from '../../assets/back.png'
import { NodeStatus, getStatus } from '../../api'
import { NetworkContext } from '../../contexts'

type StatusProps = { darkMode: boolean }

export const Status = (props: StatusProps) => {
	const navigate = useNavigate()
    const { network } = useContext(NetworkContext)
	const [version, setVersion] = useState('')
	const [nodes, setNodes] = useState<NodeStatus[]>([])
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
						    	{nodes && nodes.map(n => (
							    	<th key={'header-' + n.location}>{n.location.toUpperCase()}</th>
    							))}
	    					</tr>
		    				<tr>
			    				<th colSpan={2}>Online:</th>
				    			{nodes && nodes.map(n => (
					    			<td key={'online-' + n.location}>
						    			<div className={'status' + (n.status === true ? ' status-good' : ' status-bad')}></div>
							    	</td>
    							))}
	    					</tr>
		    				<tr>
			    				<th colSpan={2}>Version:</th>
				    			{nodes && nodes.map(n => (
                                    <td key={'version-' + n.location}>{n.status === true ? n.version : ''}</td>
						    	))}
    						</tr>
	    					<tr>
		    					<th rowSpan={2}>Mainnet:</th>
			    				<td>Height</td>
				    			{nodes && nodes.map(n => (
					    			<td key={'height-mainnet-' + n.location}>{n.status === true ? n.heightMainnet : ''}</td>
						    	))}
    						</tr>
	    					<tr>
		    					<td>Balance</td>
			    				{nodes && nodes.map(n => (
    				    			<td key={'balance-mainnet-' + n.location}>
                                        {n.status === true &&
	    				    				<div className={'status status-' + getStyle(n.balanceMainnet)}></div>
                                        }
		    				    	</td>
							    ))}
    						</tr>
	    					<tr>
		    					<th rowSpan={2}>Zen:</th>
			    				<td>Height</td>
				    			{nodes && nodes.map(n => (
					    			<td key={'height-zen-' + n.location}>{n.status === true ? n.heightZen : ''}</td>
						    	))}
    						</tr>
	    					<tr>
		    					<td>Balance</td>
			    				{nodes && nodes.map(n => (
    				    			<td key={'balance-zen-' + n.location}>
                                        {n.status === true &&
	    				    				<div className={'status status-' + getStyle(n.balanceZen)}></div>
                                        }
		    				    	</td>
							    ))}
    						</tr>
	    					<tr><td colSpan={nodes.length + 2}></td></tr>
		    				<tr>
			    				<th colSpan={2}>Portal Version:</th>
				    			<td colSpan={nodes.length}>{version}</td>
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