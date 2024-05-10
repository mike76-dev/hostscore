import './Logo.css'
import logo from '../../assets/logo.png'

export const Logo = () => {
	return (
		<a className="logo-link" href="https://hostscore.info" tabIndex={1}>
			<div className="logo-container">
				<img className="logo-image" src={logo} alt="Logo"/>
				<span className="logo-text">HostScore API</span>
			</div>
		</a>
	)
}