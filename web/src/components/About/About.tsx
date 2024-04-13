import './About.css'
import { useContext } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '../'
import Back from '../../assets/back.png'
import { NetworkContext } from '../../contexts'

type AboutProps = { darkMode: boolean }

export const About = (props: AboutProps) => {
	const navigate = useNavigate()
    const { network } = useContext(NetworkContext)
	return (
		<div className={'about-container' + (props.darkMode ? ' about-container-dark' : '')}>
			<h1>About HostScore</h1>
			<p>This site does not use any cookies or collect any user data.</p>
			<p>
				Any information found on this site can be used without any
				limitations but at the user's own risk.
				The maintainer of this site shall not take any liability for
				an eventual damage caused by any such use.
			</p>
			<p>Contact information:</p>
			<ul>
				<li>Discord: <strong>mike76</strong> (<code>mike76-dev</code>)</li>
				<li>GitHub:&nbsp;
					<a
						href="https://github.com/mike76-dev/hostscore"
						target="_blank"
						rel="noreferrer"
						tabIndex={1}
					>
						https://github.com/mike76-dev/hostscore
					</a>
				</li>
			</ul>
			<Button
				icon={Back}
				caption="back"
				darkMode={props.darkMode}
				onClick={() => {navigate(network === 'zen' ? '/zen' : '/')}}
			/>
		</div>
	)
}
