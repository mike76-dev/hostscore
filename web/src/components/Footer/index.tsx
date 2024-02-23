import './index.css'
import { ReactComponent as SiaLogo } from '../../assets/built-with-sia.svg'
import Link from '../Link'

type FooterProps = { darkMode: boolean }

const Footer = (props: FooterProps) => {
    return (
        <div className={'footer-container' + (props.darkMode ? ' footer-dark-mode' : '')}>
            <Link
                href="/about"
                caption="About"
                darkMode={props.darkMode}
            />
            <a className="footer-link" tabIndex={1} href="https://sia.tech" target="_blank">
                <SiaLogo/>
            </a>
        </div>
    )
}

export default Footer
