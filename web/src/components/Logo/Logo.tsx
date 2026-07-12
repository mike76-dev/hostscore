import './Logo.css'

export const Logo = () => {
	return (
		<a className="logo-link" href="/" tabIndex={1}>
			<span className="wordmark">
				<svg className="wordmark-hex" viewBox="0 0 26 26" aria-hidden="true">
					<polygon
						points="7.2,2.8 18.8,2.8 24.6,13 18.8,23.2 7.2,23.2 1.4,13"
						fill="none"
						stroke="var(--muted)"
						strokeWidth="1.7"
						strokeLinejoin="round"
					/>
					<text x="6" y="17.4" fontSize="11.5" fontWeight="700" fontFamily="var(--mono)" fill="var(--ink2)">H</text>
					<text x="13.4" y="17.4" fontSize="11.5" fontWeight="700" fontFamily="var(--mono)" fill="var(--accent)">S</text>
				</svg>
				<span>HOST<em>SCORE</em></span>
			</span>
		</a>
	)
}
