import './index.css'

type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & {
	icon?: string,
	caption?: string,
	darkMode: boolean,
}

const Button = (props: ButtonProps) => {
	const { icon, caption, darkMode, ...buttonProps } = props
	return (
		<button
			className={'button-container' +
			(darkMode ? ' button-container-dark' : '') +
			(props.className || '')}
			tabIndex={1}
			{...buttonProps}
		>
			<img src={icon} className="button-icon" alt=""/>
			<span className="button-caption">{caption}</span>
		</button>
	)
}

export default Button
