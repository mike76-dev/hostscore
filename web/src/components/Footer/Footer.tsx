import './Footer.css'
import { ReactComponent as SiaLogo } from '../../assets/built-with-sia.svg'
import { Link } from 'react-router-dom'

type FooterProps = { darkMode: boolean }

export const Footer = (props: FooterProps) => {
	return (
		<div className={'footer-container' + (props.darkMode ? ' footer-dark-mode' : '')}>
			<Link
				className={'footer-link' + (props.darkMode ? ' footer-link-dark' : '')}
				to="/about"
				tabIndex={1}
			>
				About
			</Link>
            <Link
				className={'footer-link' + (props.darkMode ? ' footer-link-dark' : '')}
				to="/status"
				tabIndex={1}
			>
				Status
			</Link>
			<a className="footer-link" tabIndex={1} href="https://sia.tech" target="_blank" rel="noreferrer">
				<SiaLogo/>
			</a>
		</div>
	)
}
