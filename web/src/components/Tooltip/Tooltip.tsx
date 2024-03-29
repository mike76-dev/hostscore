import './Tooltip.css'

type TooltipProps = {
	darkMode: boolean,
	children?: React.ReactNode
}

export const Tooltip = (props: TooltipProps) => {
	return (
		<div tabIndex={1} className={'tooltip-spot' + (props.darkMode ? ' tooltip-spot-dark' : '')}>?
            <div className={'tooltip-text' + (props.darkMode ? ' tooltip-text-dark' : '')}>
                {props.children}
            </div>
        </div>
	)
}
