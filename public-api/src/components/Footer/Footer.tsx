import './Footer.css'
import { ReactComponent as SiaLogo } from '../../assets/built-with-sia.svg'

type FooterProps = { darkMode: boolean }

export const Footer = (props: FooterProps) => {
	return (
		<div className={'footer-container' + (props.darkMode ? ' footer-dark-mode' : '')}>
			<a className="footer-link" tabIndex={1} href="https://sia.tech" target="_blank" rel="noreferrer">
				<SiaLogo/>
			</a>
		</div>
	)
}