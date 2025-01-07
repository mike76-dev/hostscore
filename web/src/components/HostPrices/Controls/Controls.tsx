import './Controls.css'

export type ScaleOptions = 'day' | 'week' | 'month' | 'year'

type ControlProps = {
	darkMode: boolean,
	moveLeft: () => any,
	moveRight: () => any,
	zoomIn: () => any,
	zoomOut: () => any
}

export const Controls = (props: ControlProps) => {
	return (
		<div className={'host-prices-controls-container' + (props.darkMode ? ' host-prices-dark' : '')}>
			<button className="host-prices-control" tabIndex={1} onClick={props.moveLeft}>&lt;</button>
			<button className="host-prices-control" tabIndex={1} onClick={props.moveRight}>&gt;</button>
			<button className="host-prices-control" tabIndex={1} onClick={props.zoomOut}>-</button>
			<button className="host-prices-control" tabIndex={1} onClick={props.zoomIn}>+</button>
		</div>
	)
}