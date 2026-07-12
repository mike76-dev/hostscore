import './About.css'
import { useContext, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { NetworkContext } from '../../contexts'

const donationAddress = '279ee215af98f0bcdc979714f42ecfba125cadbda1ba3dada716f4de1718267db949a7e5040c'

type AboutProps = { darkMode: boolean }

export const About = (props: AboutProps) => {
	const navigate = useNavigate()
	const { network } = useContext(NetworkContext)
	const [copied, setCopied] = useState(false)
	const copyAddress = () => {
		if (!navigator.clipboard) return
		navigator.clipboard.writeText(donationAddress)
		.then(() => {
			setCopied(true)
			setTimeout(() => setCopied(false), 1500)
		})
	}
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
			<p>HostScore was initially funded by the&nbsp;
				<a
					href="https://sia.tech/grants"
					target="_blank"
					rel="noreferrer"
					tabIndex={1}
				>
					Sia Grants Program
				</a>. However, it still requires private funding to operate the infrastructure.
				You can support the project by donating some Siacoin to
			</p>
			<div className="about-donation">
				<input
					type="text"
					readOnly
					tabIndex={1}
					value={donationAddress}
				/>
				<button
					className="copy-btn"
					tabIndex={1}
					onClick={copyAddress}
				>{copied ? 'COPIED' : 'COPY'}</button>
			</div>
			<button
				className="button-container"
				tabIndex={1}
				onClick={() => {navigate(network === 'zen' ? '/zen' : '/')}}
			>← home</button>
		</div>
	)
}
