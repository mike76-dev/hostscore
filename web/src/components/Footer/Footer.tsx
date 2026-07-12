import './Footer.css'
import { Link } from 'react-router-dom'
import { ReactComponent as BadgeDark } from '../../assets/built-on-sia-primary-dark.svg'
import { ReactComponent as BadgeLight } from '../../assets/built-on-sia-primary-light.svg'

type FooterProps = { darkMode: boolean }

export const Footer = (props: FooterProps) => {
	return (
		<footer className="footer-container">
			<div className="wrap footer-inner">
				<span className="wordmark footer-wordmark"><span>HOST<em>SCORE</em></span></span>
				<nav className="footer-links" aria-label="Footer">
					<Link className="footer-link" to="/about" tabIndex={1}>About</Link>
					<Link className="footer-link" to="/faq" tabIndex={1}>FAQ</Link>
					<Link className="footer-link" to="/status" tabIndex={1}>Status</Link>
					<a
						className="footer-link"
						tabIndex={1}
						href="https://github.com/mike76-dev/hostscore"
						target="_blank"
						rel="noreferrer"
					>GitHub</a>
					<a
						className="footer-link"
						tabIndex={1}
						href="https://api.hostscore.info"
						target="_blank"
						rel="noreferrer"
					>API docs</a>
				</nav>
				<a
					className="footer-badge"
					tabIndex={1}
					href="https://sia.tech"
					target="_blank"
					rel="noreferrer"
					aria-label="Built on Sia"
				>
					{props.darkMode ? <BadgeLight/> : <BadgeDark/>}
				</a>
			</div>
		</footer>
	)
}
