import './Tooltip.css'

type TooltipProps = {
	darkMode: boolean,
	className?: string,
	children?: React.ReactNode
}

export const Tooltip = (props: TooltipProps) => {
	return (
		<div tabIndex={1} className={'tooltip-spot' + (props.darkMode ? ' tooltip-spot-dark ' : ' ') + (props.className || '')}>?
			<div className={'tooltip-text' + (props.darkMode ? ' tooltip-text-dark' : '')}>
				{props.children}
			</div>
		</div>
	)
}
